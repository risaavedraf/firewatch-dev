package main

import (
	"net/http"
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
	// so it is the only URL these tests may treat as acceptable outside development.
	encryptedDatabaseURL = "postgres://user:pass@db:5432/x?sslmode=verify-full"
	// sslmode=require encrypts without checking who answered, and sslmode=prefer
	// keeps a plaintext fallback. Both look encrypted at a glance and are not.
	unauthenticatedDatabaseURL = "postgres://user:pass@db:5432/x?sslmode=require"
	fallbackDatabaseURL        = "postgres://user:pass@db:5432/x?sslmode=prefer"
	plaintextBrokerURL         = "amqp://user:pass@broker:5672/firewatch"
	encryptedBrokerURL         = "amqps://user:pass@broker:5671/firewatch"
	unverifiedBrokerURL        = "amqps://user:pass@broker:5671/firewatch?verify=verify_none"

	tlsGuardMessage    = "DATABASE_URL must require TLS outside development"
	tlsFallbackMessage = "DATABASE_URL must not fall back to a plaintext connection outside development"
	tlsVerifyMessage   = "DATABASE_URL must verify the server certificate outside development"
	amqpsGuardMessage  = "AMQP_URL must use amqps outside development"
	verifyGuardMessage = "AMQP_URL must not disable certificate verification outside development"

	// certPath and keyPath are the mount points the crossing contract names. The
	// guard reads only their presence; the listener tests are the ones that read
	// the files themselves.
	certPath = "/etc/tls/tls.crt"
	keyPath  = "/etc/tls/tls.key"

	certMissingOutsideDevelopment = "TLS_CERT_FILE is required outside development: set TLS_CERT_FILE and TLS_KEY_FILE, or run with APP_ENV=development for plaintext"
	keyMissingForConfiguredCert   = "TLS_KEY_FILE is required when TLS_CERT_FILE is set"
	certMissingForConfiguredKey   = "TLS_CERT_FILE is required when TLS_KEY_FILE is set"
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

// assertGuardResult states the observable contract of every guard in this file:
// either it accepts the configuration or it refuses it with exactly the message
// the operator will read. An empty want means the guard must accept.
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

// TestRequireSecureBrokerURL pins the broker rule, including the "do not verify"
// switch: amqps with verification disabled protects nothing against an active
// attacker, so it is refused wherever plaintext is refused.
func TestRequireSecureBrokerURL(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment string
		brokerURL   string
		want        string
	}{
		{name: "plaintext outside development", environment: "production", brokerURL: plaintextBrokerURL, want: amqpsGuardMessage},
		{name: "amqps outside development", environment: "production", brokerURL: encryptedBrokerURL},
		{name: "plaintext in development", environment: "development", brokerURL: plaintextBrokerURL},
		{name: "plaintext with an unset environment", environment: "", brokerURL: plaintextBrokerURL, want: amqpsGuardMessage},
		{name: "the development exception is not case-insensitive", environment: "Development", brokerURL: plaintextBrokerURL, want: amqpsGuardMessage},
		{name: "verification disabled outside development", environment: "production", brokerURL: unverifiedBrokerURL, want: verifyGuardMessage},
		{name: "verification disabled in development", environment: "development", brokerURL: unverifiedBrokerURL},
		{name: "unparsable URL outside development", environment: "production", brokerURL: "amqps://user:pass@broker:5671/%zz", want: amqpsGuardMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGuardResult(t, requireSecureBrokerURL(tc.brokerURL, tc.environment), tc.want)
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
	t.Setenv("AMQP_URL", encryptedBrokerURL)
	assertGuardResult(t, run(), tlsGuardMessage)
}

// TestStartupBrokerURLRefusesPlaintextOutsideDevelopment pins the broker guard to
// the read the worker is built from. The migrate path returns before this read,
// which is what keeps a migration job that holds only DATABASE_URL working.
func TestStartupBrokerURLRefusesPlaintextOutsideDevelopment(t *testing.T) {
	t.Setenv("AMQP_URL", plaintextBrokerURL)
	_, err := startupBrokerURL("production")
	assertGuardResult(t, err, amqpsGuardMessage)
}

// TestRequireListenerTLS pins T7.3's inbound half of the crossing contract: a
// configured key pair serves TLS; no key pair keeps the plaintext of development
// and nothing else; every other combination refuses to start and names the
// variable that is missing, including a half pair in development.
func TestRequireListenerTLS(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment string
		certFile    string
		keyFile     string
		want        string
	}{
		{name: "key pair outside development", environment: "staging", certFile: certPath, keyFile: keyPath},
		{name: "key pair in production", environment: "production", certFile: certPath, keyFile: keyPath},
		{name: "key pair in development", environment: "development", certFile: certPath, keyFile: keyPath},
		{name: "no key pair in development", environment: "development"},
		{name: "no key pair outside development", environment: "staging", want: certMissingOutsideDevelopment},
		{name: "no key pair with an unset environment", want: certMissingOutsideDevelopment},
		{name: "the development exception is not case-insensitive", environment: "Development", want: certMissingOutsideDevelopment},
		{name: "certificate without a key outside development", environment: "staging", certFile: certPath, want: keyMissingForConfiguredCert},
		{name: "key without a certificate outside development", environment: "staging", keyFile: keyPath, want: certMissingForConfiguredKey},
		{name: "certificate without a key in development", environment: "development", certFile: certPath, want: keyMissingForConfiguredCert},
		{name: "key without a certificate in development", environment: "development", keyFile: keyPath, want: certMissingForConfiguredKey},
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
	t.Setenv(tlsCertFileEnv, certPath)
	t.Setenv(tlsKeyFileEnv, keyPath)
	cert, key, err := startupListenerTLS("staging")
	if err != nil || cert != certPath || key != keyPath {
		t.Fatalf("configured key pair = %q, %q, %v", cert, key, err)
	}
	t.Setenv(tlsCertFileEnv, "")
	t.Setenv(tlsKeyFileEnv, "")
	if _, _, err = startupListenerTLS("production"); err == nil || !strings.Contains(err.Error(), tlsCertFileEnv) {
		t.Fatalf("startup outside development without TLS = %v, want a refusal naming %s", err, tlsCertFileEnv)
	}
	// The development plaintext case is the only accepted one that serves without
	// a key pair, and it resolves to no certificate and no key.
	if cert, key, err = startupListenerTLS("development"); err != nil || cert != "" || key != "" {
		t.Fatalf("development without TLS = %q, %q, %v", cert, key, err)
	}
}

// TestStartupRootCAsLoadsTheInternalCA pins T7.3's outbound half: no TLS_CA_FILE
// is no pool, so every client keeps the system roots it uses today, and a
// configured bundle is loaded from disk. A bundle that cannot be trusted is a
// refusal that names the variable, never a silent fall back to the system roots.
func TestStartupRootCAsLoadsTheInternalCA(t *testing.T) {
	t.Setenv(tlsCAFileEnv, "")
	if pool, err := startupRootCAs(); err != nil || pool != nil {
		t.Fatalf("no CA = %v, %v; want no pool and the system roots", pool, err)
	}
	_, _, caFile := testTLSKeyPair(t)
	t.Setenv(tlsCAFileEnv, caFile)
	if pool, err := startupRootCAs(); err != nil || pool == nil {
		t.Fatalf("configured CA = %v, %v; want a loaded pool", pool, err)
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
			_, err := startupRootCAs()
			if err == nil || !strings.Contains(err.Error(), tlsCAFileEnv) {
				t.Fatalf("startup with %s = %v, want a refusal naming %s", tc.path, err, tlsCAFileEnv)
			}
		})
	}
}

// TestNewJWKSClientCarriesTheInternalCA pins the key-set client of T7.3: the
// configured pool is the trust anchor it verifies the issuer with, and without
// TLS_CA_FILE it stays the bare client with the system roots - no Transport at
// all, so an unset variable cannot reshape the connection.
func TestNewJWKSClientCarriesTheInternalCA(t *testing.T) {
	_, _, caFile := testTLSKeyPair(t)
	t.Setenv(tlsCAFileEnv, caFile)
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
