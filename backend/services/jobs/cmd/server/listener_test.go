package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestListenerServesPlaintextInDevelopment covers the one case that may still
// serve in the clear: development with neither half of the key pair configured.
// The listener is real and so is the request, so this proves the plaintext path
// still works instead of only proving the guard accepts the configuration.
func TestListenerServesPlaintextInDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv(tlsCertFileEnv, "")
	t.Setenv(tlsKeyFileEnv, "")
	certFile, keyFile, err := startupListenerTLS(os.Getenv("APP_ENV"))
	if err != nil || certFile != "" || keyFile != "" {
		t.Fatalf("development transport = %q, %q, %v", certFile, keyFile, err)
	}
	served, addr := serveForTest(t, certFile, keyFile)
	response := waitForHTTP(t, &http.Client{Timeout: 5 * time.Second}, "http://"+addr+"/health/live", served)
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("plaintext /health/live got %d: %q", response.StatusCode, body)
	}
}

// TestListenerServesTLSWithTheConfiguredKeyPair is the acceptance test of T7.2's
// inbound half: with a certificate and a key, the listener answers a client that
// trusts the CA that signed them, so what runs is TLS and not a plaintext
// listener behind an https:// URL. The same request with the system roots must
// fail, so the success cannot be explained by a skipped verification.
func TestListenerServesTLSWithTheConfiguredKeyPair(t *testing.T) {
	certFile, keyFile, caFile := testKeyPair(t)
	t.Setenv(tlsCAFileEnv, caFile)
	roots, err := startupRootCAs()
	if err != nil || roots == nil {
		t.Fatalf("the generated CA did not load: %v", err)
	}
	served, addr := serveForTest(t, certFile, keyFile)
	trusted := &http.Client{
		Timeout: 5 * time.Second,
		// The certificate's only subject alternative name is localhost, which is
		// what the in-cluster callers dial; the test connects to loopback.
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost"}},
	}
	response := waitForHTTP(t, trusted, "https://"+addr+"/health/live", served)
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("TLS /health/live got %d: %q", response.StatusCode, body)
	}
	untrusted := &http.Client{Timeout: 5 * time.Second}
	if response, err := untrusted.Get("https://" + addr + "/health/live"); err == nil {
		response.Body.Close()
		t.Fatalf("a client with the system roots got %d, want a certificate verification failure", response.StatusCode)
	}
}

// serveForTest starts the listener the process serves, on a free loopback port so
// the test never competes for 8080, and stops it when the test ends.
func serveForTest(t *testing.T, certFile, keyFile string) (<-chan error, string) {
	t.Helper()
	// The port is reserved and released before the server binds it: ListenAndServe
	// and ListenAndServeTLS take an address, not a net.Listener, and the window is
	// the smallest one this shape allows.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	addr := probe.Addr().String()
	if err = probe.Close(); err != nil {
		t.Fatalf("release the reserved port: %v", err)
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }),
		ReadHeaderTimeout: 5 * time.Second,
	}
	served := make(chan error, 1)
	go func() { served <- listenAndServe(server, certFile, keyFile) }()
	t.Cleanup(func() { _ = server.Close() })
	return served, addr
}

// waitForHTTP retries until the listener answers or the deadline passes, and
// reports a listener that stopped on its own instead of waiting out the deadline.
func waitForHTTP(t *testing.T, client *http.Client, url string, served <-chan error) *http.Response {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-served:
			t.Fatalf("the listener stopped before answering %s: %v", url, err)
		default:
		}
		response, err := client.Get(url)
		if err == nil {
			return response
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no answer from %s within 5s: %v", url, lastErr)
	return nil
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

// testKeyPair writes a throwaway CA and a server certificate signed by it into
// the test's own temp directory and returns the paths the listener reads plus the
// path of the CA bundle startupRootCAs loads. Nothing is shared with the
// repository or the deployment: the real secrets are issued by
// bootstrap-secrets.sh.
func testKeyPair(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "firewatch-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca certificate: %v", err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "jobs"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("server certificate: %v", err)
	}
	serverKeyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatalf("server key encoding: %v", err)
	}
	dir := t.TempDir()
	certFile, keyFile, caFile = filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"), filepath.Join(dir, "ca.crt")
	if err = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverKeyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err = os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}
	return certFile, keyFile, caFile
}
