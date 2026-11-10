package main

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	plaintextDatabaseURL = "postgres://user:pass@db:5432/x?sslmode=disable"
	// verify-full is the only mode that both encrypts and authenticates the server,
	// so it is the only URL this test may treat as acceptable outside development.
	encryptedDatabaseURL       = "postgres://user:pass@db:5432/x?sslmode=verify-full"
	unauthenticatedDatabaseURL = "postgres://user:pass@db:5432/x?sslmode=require"
	fallbackDatabaseURL        = "postgres://user:pass@db:5432/x?sslmode=prefer"
	tlsGuardMessage            = "DATABASE_URL must require TLS outside development"
	tlsFallbackMessage         = "DATABASE_URL must not fall back to a plaintext connection outside development"
	tlsVerifyMessage           = "DATABASE_URL must verify the server certificate outside development"
)

// databaseConfigFor builds the pool configuration from a URL exactly as startup
// does, so the guard is exercised against what pgx derives from sslmode instead
// of a hand-made struct that proves nothing about the parsed value.
func databaseConfigFor(t *testing.T, databaseURL string) *pgxpool.Config {
	t.Helper()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse %q: %v", databaseURL, err)
	}
	return config
}

// assertGuardResult states the observable contract of the guard: either it
// accepts the configuration or it refuses it with exactly the message the
// operator will read. An empty want means the guard must accept.
func assertGuardResult(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("the guard refused a configuration it must accept: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("the guard accepted a configuration it must refuse, want %q", want)
	}
	if err.Error() != want {
		t.Fatalf("the guard refused with %q, want %q", err.Error(), want)
	}
}

// TestRequireTLSOutsideDevelopment pins the database rule: a URL that negotiates
// no TLS is a plaintext connection, and the only exception is the literal
// environment value "development". An unset or differently spelled value has to
// fail closed, because that spelling is what a deployment gets wrong.
func TestRequireTLSOutsideDevelopment(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment string
		databaseURL string
		want        string
	}{
		{name: "plaintext outside development", environment: "production", databaseURL: plaintextDatabaseURL, want: tlsGuardMessage},
		{name: "verified tls outside development", environment: "production", databaseURL: encryptedDatabaseURL},
		// Encrypted but unauthenticated, and encrypted with a way out: both are
		// refusals, because either one lets an active attacker succeed.
		{name: "require without verification outside development", environment: "production", databaseURL: unauthenticatedDatabaseURL, want: tlsVerifyMessage},
		{name: "prefer with a plaintext fallback outside development", environment: "production", databaseURL: fallbackDatabaseURL, want: tlsFallbackMessage},
		{name: "plaintext in development", environment: "development", databaseURL: plaintextDatabaseURL},
		{name: "plaintext with an unset environment", environment: "", databaseURL: plaintextDatabaseURL, want: tlsGuardMessage},
		{name: "the development exception is not case-insensitive", environment: "Development", databaseURL: plaintextDatabaseURL, want: tlsGuardMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGuardResult(t, requireTLSOutsideDevelopment(databaseConfigFor(t, tc.databaseURL), tc.environment), tc.want)
		})
	}
}

// TestRunRefusesPlaintextDatabaseOutsideDevelopment pins the wiring, not only
// the guard: the process stops on DATABASE_URL before it opens the pool, so a
// production start cannot reach a connection attempt. The address is a closed
// loopback port, so even a run that got as far as the pool fails immediately
// instead of reaching for a database that might exist.
func TestRunRefusesPlaintextDatabaseOutsideDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://user:pass@127.0.0.1:1/x?sslmode=disable")
	assertGuardResult(t, run(), tlsGuardMessage)
}

// The three messages the operator can read for the listener, and the paths the
// guard is given. Naming the variable is the point: an operator with a
// half-mounted secret must not have to guess which half is missing.
const (
	listenerCertMissingOutsideDevelopment = "TLS_CERT_FILE is required outside development: set TLS_CERT_FILE and TLS_KEY_FILE, or run with APP_ENV=development for plaintext"
	listenerCertMissingForConfiguredKey   = "TLS_CERT_FILE is required when TLS_KEY_FILE is set"
	listenerKeyMissingForConfiguredCert   = "TLS_KEY_FILE is required when TLS_CERT_FILE is set"
	// Any path works here: the guard decides from the strings alone, and the
	// listener is the only place that opens the files.
	listenerCertPath = "/etc/tls/tls.crt"
	listenerKeyPath  = "/etc/tls/tls.key"
)

// TestRequireListenerTLS pins T7.4's inbound half of the crossing contract
// (odd/tasks/kubernetes-deployment.md, L10): a configured key pair serves TLS;
// no key pair keeps the plaintext of development and nothing else; every other
// combination refuses to start and names the variable that is missing,
// including a half pair in development.
func TestRequireListenerTLS(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment string
		certFile    string
		keyFile     string
		want        string
	}{
		{name: "key pair outside development", environment: "staging", certFile: listenerCertPath, keyFile: listenerKeyPath},
		{name: "key pair in production", environment: "production", certFile: listenerCertPath, keyFile: listenerKeyPath},
		{name: "key pair in development", environment: "development", certFile: listenerCertPath, keyFile: listenerKeyPath},
		{name: "no key pair in development", environment: "development"},
		{name: "no key pair outside development", environment: "staging", want: listenerCertMissingOutsideDevelopment},
		{name: "no key pair with an unset environment", want: listenerCertMissingOutsideDevelopment},
		{name: "the development exception is not case-insensitive", environment: "Development", want: listenerCertMissingOutsideDevelopment},
		{name: "certificate without a key outside development", environment: "staging", certFile: listenerCertPath, want: listenerKeyMissingForConfiguredCert},
		{name: "key without a certificate outside development", environment: "staging", keyFile: listenerKeyPath, want: listenerCertMissingForConfiguredKey},
		{name: "certificate without a key in development", environment: "development", certFile: listenerCertPath, want: listenerKeyMissingForConfiguredCert},
		{name: "key without a certificate in development", environment: "development", keyFile: listenerKeyPath, want: listenerCertMissingForConfiguredKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGuardResult(t, requireListenerTLS(tc.environment, tc.certFile, tc.keyFile), tc.want)
		})
	}
}

// TestStartupListenerTLSTakesTheDecisionFromTheEnvironment pins the wiring, not
// only the guard: startup reads the two variables, and the values it accepts are
// exactly the ones handed to the listener.
func TestStartupListenerTLSTakesTheDecisionFromTheEnvironment(t *testing.T) {
	t.Setenv(tlsCertFileEnv, listenerCertPath)
	t.Setenv(tlsKeyFileEnv, listenerKeyPath)
	cert, key, err := startupListenerTLS("staging")
	if err != nil || cert != listenerCertPath || key != listenerKeyPath {
		t.Fatalf("configured key pair = %q, %q, %v", cert, key, err)
	}
	t.Setenv(tlsCertFileEnv, "")
	t.Setenv(tlsKeyFileEnv, "")
	if _, _, err = startupListenerTLS("production"); err == nil || !strings.Contains(err.Error(), tlsCertFileEnv) {
		t.Fatalf("startup outside development without TLS = %v, want a refusal naming %s", err, tlsCertFileEnv)
	}
	// The development plaintext case is the only accepted one that serves
	// without a key pair, and it resolves to no certificate and no key.
	if cert, key, err = startupListenerTLS("development"); err != nil || cert != "" || key != "" {
		t.Fatalf("development without TLS = %q, %q, %v", cert, key, err)
	}
}

// freeAddr reserves and releases a loopback port so a real listener can bind it
// in the same test. The window between the two is what makes this a probe and
// not a reservation, so the serving tests retry instead of assuming the bind
// succeeded on the first attempt.
func freeAddr(t *testing.T) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a loopback port: %v", err)
	}
	address := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release the loopback port: %v", err)
	}
	return address
}

// TestDevelopmentWithoutTLSKeepsServingPlaintext pins the behaviour that must
// not break: in development with no key pair the service still binds 8084's
// address and answers over plaintext, exactly as it did before the TLS work.
func TestDevelopmentWithoutTLSKeepsServingPlaintext(t *testing.T) {
	// Both halves are cleared so an exported TLS_CERT_FILE in the surrounding
	// shell cannot turn this into the TLS branch it is not testing.
	t.Setenv(tlsCertFileEnv, "")
	t.Setenv(tlsKeyFileEnv, "")
	certFile, keyFile, err := startupListenerTLS("development")
	if err != nil {
		t.Fatalf("development without TLS = %v, want the plaintext exception", err)
	}
	server := &http.Server{
		Addr:              freeAddr(t),
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		ReadHeaderTimeout: 5 * time.Second,
	}
	served := make(chan error, 1)
	go func() { served <- listenAndServe(server, certFile, keyFile) }()
	t.Cleanup(func() {
		_ = server.Close()
		<-served
	})

	client := &http.Client{Timeout: time.Second}
	address := "http://" + server.Addr + "/health/live"
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := client.Get(address)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				t.Fatalf("plaintext prevention answered %d, want %d", response.StatusCode, http.StatusNoContent)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the development prevention service never answered in the clear: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestListenAndServeTakesTheTLSBranchForAConfiguredPair pins that a configured
// pair is served by ListenAndServeTLS: with an unreadable certificate the call
// must return the file error, and the plaintext branch would instead keep
// serving and never report it.
func TestListenAndServeTakesTheTLSBranchForAConfiguredPair(t *testing.T) {
	certFile := filepath.Join(t.TempDir(), "absent.crt")
	server := &http.Server{Addr: freeAddr(t), ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- listenAndServe(server, certFile, filepath.Join(t.TempDir(), "absent.key")) }()
	t.Cleanup(func() { _ = server.Close() })

	select {
	case err := <-served:
		if err == nil || !strings.Contains(err.Error(), certFile) {
			t.Fatalf("a configured key pair with an absent certificate = %v, want the file error naming %s", err, certFile)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the listener kept serving in the clear instead of taking the TLS branch")
	}
}

// caBundleFromServer writes the certificate a test server presents as the PEM
// bundle TLS_CA_FILE names, inside a fresh temp dir.
func caBundleFromServer(t *testing.T, server *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	bundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(path, bundle, 0o600); err != nil {
		t.Fatalf("write the CA bundle: %v", err)
	}
	return path
}

// TestStartupRootCAsLoadsTheInternalCA pins T7.4's outbound trust: no
// TLS_CA_FILE is no pool, so every client keeps the system roots it uses today,
// and a configured bundle is loaded and actually vouches for its certificate. A
// bundle that cannot be trusted is a refusal that names the variable, never a
// silent fall back to the system roots.
func TestStartupRootCAsLoadsTheInternalCA(t *testing.T) {
	t.Setenv(tlsCAFileEnv, "")
	if pool, err := startupRootCAs(); err != nil || pool != nil {
		t.Fatalf("no TLS_CA_FILE = %v, %v; want the system roots", pool, err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	t.Setenv(tlsCAFileEnv, caBundleFromServer(t, server))
	pool, err := startupRootCAs()
	if err != nil || pool == nil {
		t.Fatalf("configured TLS_CA_FILE = %v, %v", pool, err)
	}
	if _, err := server.Certificate().Verify(x509.VerifyOptions{Roots: pool, DNSName: "127.0.0.1"}); err != nil {
		t.Fatalf("the loaded pool does not vouch for the bundle it was built from: %v", err)
	}
	dir := t.TempDir()
	blank := filepath.Join(dir, "blank.crt")
	if err := os.WriteFile(blank, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write blank bundle: %v", err)
	}
	for _, tc := range []struct{ name, path string }{
		{"missing file", filepath.Join(dir, "absent.crt")},
		{"bundle without a certificate", blank},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tlsCAFileEnv, tc.path)
			if _, err := startupRootCAs(); err == nil || !strings.Contains(err.Error(), tlsCAFileEnv) {
				t.Fatalf("startup with %s = %v, want a refusal naming %s", tc.path, err, tlsCAFileEnv)
			}
		})
	}
}

// TestNewJWKSClientCarriesTheInternalCA pins the key-set client: the configured
// pool is the trust anchor the issuer is verified with, and without TLS_CA_FILE
// it stays the bare client with the system roots - no Transport at all, so an
// unset variable cannot reshape the connection. The 5s bound is kept in both.
func TestNewJWKSClientCarriesTheInternalCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	t.Setenv(tlsCAFileEnv, caBundleFromServer(t, server))
	pool, err := startupRootCAs()
	if err != nil {
		t.Fatalf("load CA: %v", err)
	}
	client := newJWKSClient(pool)
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs != pool {
		t.Fatalf("the key-set client does not carry the configured CA: %#v", client.Transport)
	}
	t.Setenv(tlsCAFileEnv, "")
	if pool, err = startupRootCAs(); err != nil {
		t.Fatalf("load no CA: %v", err)
	}
	bare := newJWKSClient(pool)
	if bare.Transport != nil {
		t.Fatalf("without TLS_CA_FILE the key-set client must keep no Transport, got %#v", bare.Transport)
	}
	for _, c := range []*http.Client{client, bare} {
		if c.Timeout != 5*time.Second {
			t.Fatalf("key-set client timeout %v, want the 5s bound it always had", c.Timeout)
		}
	}
}
