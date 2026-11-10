package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// secureUpstreams is the configuration a production gateway must be given; every
// case below starts from it and changes only the value under test, so a failure
// names the upstream that made the hop plaintext.
func secureUpstreams() map[string]string {
	return map[string]string{
		"JOBS_URL":       "https://jobs:8080",
		"RISK_URL":       "https://risk:8080",
		"PREVENTION_URL": "https://prevention:8080",
	}
}

// plaintextUpstreams is the development configuration this guard has to keep
// accepting, because the local stack runs every service over plain HTTP.
func plaintextUpstreams() map[string]string {
	return map[string]string{
		"JOBS_URL":       "http://jobs:8080",
		"RISK_URL":       "http://risk:8090",
		"PREVENTION_URL": "http://prevention:8084",
	}
}

// withUpstream returns the secure set with one value replaced.
func withUpstream(name, value string) map[string]string {
	upstreams := secureUpstreams()
	upstreams[name] = value
	return upstreams
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

// TestRequireSecureUpstreams pins the gateway rule: outside development every
// upstream it proxies to must be https, and the refusal names the variable the
// operator has to change. The only exception is the literal "development"; an
// unset or differently spelled environment fails closed.
func TestRequireSecureUpstreams(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment string
		upstreams   map[string]string
		want        string
	}{
		{name: "https outside development", environment: "production", upstreams: secureUpstreams()},
		{name: "plaintext jobs outside development", environment: "production", upstreams: withUpstream("JOBS_URL", "http://jobs:8080"), want: "JOBS_URL must use https outside development"},
		{name: "plaintext prevention outside development", environment: "production", upstreams: withUpstream("PREVENTION_URL", "http://prevention:8084"), want: "PREVENTION_URL must use https outside development"},
		{name: "every plaintext upstream names the same stable offender", environment: "production", upstreams: plaintextUpstreams(), want: "JOBS_URL must use https outside development"},
		{name: "unparsable upstream outside development", environment: "production", upstreams: withUpstream("RISK_URL", "https://risk:8080/%zz"), want: "RISK_URL must use https outside development"},
		{name: "plaintext in development", environment: "development", upstreams: plaintextUpstreams()},
		{name: "plaintext with an unset environment", environment: "", upstreams: plaintextUpstreams(), want: "JOBS_URL must use https outside development"},
		{name: "the development exception is not case-insensitive", environment: "Development", upstreams: plaintextUpstreams(), want: "JOBS_URL must use https outside development"},
		// An empty upstream never reaches this guard in the running gateway: its
		// own configuration step rejects a missing URL first. Skipping it keeps that
		// message and this guard's scope unchanged.
		{name: "empty upstreams are not refused here", environment: "production", upstreams: map[string]string{"JOBS_URL": "", "RISK_URL": "", "PREVENTION_URL": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGuardResult(t, requireSecureUpstreams(tc.environment, tc.upstreams), tc.want)
		})
	}
}

// TestStartupTransportRefusesPlaintextUpstreamsOutsideDevelopment pins the wiring:
// startup reads the upstream variables and returns the refusal, so the process
// stops before the proxy is built and before the listener is opened.
func TestStartupTransportRefusesPlaintextUpstreamsOutsideDevelopment(t *testing.T) {
	t.Setenv("JOBS_URL", "http://jobs:8080")
	t.Setenv("RISK_URL", "https://risk:8080")
	t.Setenv("PREVENTION_URL", "https://prevention:8080")
	assertGuardResult(t, startupTransport("production"), "JOBS_URL must use https outside development")
}

// The three messages the operator can read for the listener. They are the ones
// every service of the deployment emits (odd/tasks/kubernetes-deployment.md,
// L10), so an operator reading two services' logs sees one wording.
const (
	listenerCertMissingOutsideDevelopment = "TLS_CERT_FILE is required outside development: set TLS_CERT_FILE and TLS_KEY_FILE, or run with APP_ENV=development for plaintext"
	listenerCertMissingForConfiguredKey   = "TLS_CERT_FILE is required when TLS_KEY_FILE is set"
	listenerKeyMissingForConfiguredCert   = "TLS_KEY_FILE is required when TLS_CERT_FILE is set"
	// Any path works here: the guard decides from the strings alone, and the
	// listener is the only place that opens the files.
	listenerCertPath = "/etc/tls/tls.crt"
	listenerKeyPath  = "/etc/tls/tls.key"
)

// TestRequireListenerTLS pins the crossing contract of T7.1: a configured key
// pair serves TLS; no key pair keeps the plaintext of development and nothing
// else; every other combination refuses to start and names the variable that is
// missing. A half-configured pair is refused even in development, because a
// mount that lost one half is a configuration error, not a reason to fall back
// to plaintext.
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
		// The development exception is the literal value the local stack sets and
		// only with neither half configured. An unset or differently spelled
		// APP_ENV describes a deployment, and a deployment must not serve plaintext.
		{name: "no key pair in development", environment: "development"},
		{name: "no key pair outside development", environment: "staging", want: listenerCertMissingOutsideDevelopment},
		{name: "no key pair with an unset environment", want: listenerCertMissingOutsideDevelopment},
		{name: "the development exception is not case-insensitive", environment: "Development", want: listenerCertMissingOutsideDevelopment},
		{name: "the development exception does not apply to production", environment: "production", want: listenerCertMissingOutsideDevelopment},
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
	if _, _, err := startupListenerTLS("production"); err == nil || !strings.Contains(err.Error(), tlsCertFileEnv) {
		t.Fatalf("startup outside development without TLS = %v, want a refusal naming %s", err, tlsCertFileEnv)
	}

	// The development plaintext case is the only accepted one without a key pair,
	// and it resolves to no certificate and no key.
	cert, key, err = startupListenerTLS("development")
	if err != nil || cert != "" || key != "" {
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
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release the loopback port: %v", err)
	}
	return addr
}

// TestDevelopmentWithoutTLSKeepsServingPlaintext pins the behaviour that must
// not break: in development with no key pair the gateway still binds and answers
// over plaintext, exactly as it did before the TLS work.
func TestDevelopmentWithoutTLSKeepsServingPlaintext(t *testing.T) {
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
	url := "http://" + server.Addr + "/health/live"
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				t.Fatalf("plaintext gateway answered %d, want %d", response.StatusCode, http.StatusNoContent)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the development gateway never answered in the clear: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestListenAndServeTakesTheTLSBranchForAConfiguredPair pins that a configured
// pair is served by ListenAndServeTLS: with an unreadable certificate the call
// must return the file error, and a plaintext branch would instead keep serving
// and never report it.
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

// TestStartupRootCAsKeepsSystemRootsWithoutTLSCAFile pins the no-CA case of the
// trust side: no path is no pool, so every outbound client keeps the system
// roots it uses today.
func TestStartupRootCAsKeepsSystemRootsWithoutTLSCAFile(t *testing.T) {
	t.Setenv(tlsCAFileEnv, "")
	pool, err := startupRootCAs()
	if err != nil || pool != nil {
		t.Fatalf("no TLS_CA_FILE = %v, %v; want the system roots", pool, err)
	}
}

// TestStartupRootCAsLoadsTheConfiguredBundle pins the trust side: the PEM named
// by TLS_CA_FILE becomes the pool, and that pool actually vouches for the
// certificate the bundle holds.
func TestStartupRootCAsLoadsTheConfiguredBundle(t *testing.T) {
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
}

// TestStartupRootCAsNamesTheVariableWhenTheBundleIsUnusable pins the refusal: a
// set-but-unreadable or certificate-free bundle stops startup and names the
// variable, instead of quietly falling back to the system roots.
func TestStartupRootCAsNamesTheVariableWhenTheBundleIsUnusable(t *testing.T) {
	garbage := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(garbage, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write the unusable bundle: %v", err)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "absent.crt"), garbage} {
		t.Setenv(tlsCAFileEnv, path)
		if pool, err := startupRootCAs(); err == nil || pool != nil || !strings.Contains(err.Error(), tlsCAFileEnv) {
			t.Fatalf("%s = %v, %v; want a refusal naming %s", path, pool, err, tlsCAFileEnv)
		}
	}
}

// TestJWKSClientTrustsOnlyTheConfiguredCA is the negotiation for the outbound
// identity client: given the pool from TLS_CA_FILE it reaches a key endpoint
// whose certificate only that CA vouches for, keeps its five second timeout, and
// without a CA it is exactly the bare client this process used before, which is
// what makes the absence of trust visible instead of silent.
func TestJWKSClientTrustsOnlyTheConfiguredCA(t *testing.T) {
	keys := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"keys":[]}`)
	}))
	defer keys.Close()
	t.Setenv(tlsCAFileEnv, caBundleFromServer(t, keys))
	rootCAs, err := startupRootCAs()
	if err != nil || rootCAs == nil {
		t.Fatalf("configured TLS_CA_FILE = %v, %v", rootCAs, err)
	}

	client := newJWKSClient(rootCAs)
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs != rootCAs {
		t.Fatalf("the JWKS client does not carry the configured pool: %+v", client.Transport)
	}
	if client.Timeout != 5*time.Second {
		t.Fatalf("the JWKS timeout = %v, want 5s kept", client.Timeout)
	}
	response, err := client.Get(keys.URL)
	if err != nil {
		t.Fatalf("the JWKS client does not trust the CA it was given: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the JWKS endpoint answered %d, want %d", response.StatusCode, http.StatusOK)
	}

	bare := newJWKSClient(nil)
	if bare.Transport != nil || bare.Timeout != 5*time.Second {
		t.Fatalf("the client without a CA changed: transport %+v, timeout %v", bare.Transport, bare.Timeout)
	}
	if _, err := bare.Get(keys.URL); err == nil {
		t.Fatal("the client without a CA trusted a certificate no system root vouches for")
	}
}
