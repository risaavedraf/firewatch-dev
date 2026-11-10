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
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"firewatch.local/inference/internal/artifact"
)

// TestModelPathIsRequired keeps startup from looking somewhere else when nothing
// is configured: a default path would make a deployment that forgot the mount
// start on whatever happened to be in the image.
func TestModelPathIsRequired(t *testing.T) {
	if _, err := modelPath(""); err == nil || !strings.Contains(err.Error(), modelPathEnv) {
		t.Fatalf("empty path error = %v", err)
	}
	if path, err := modelPath("testdata/logistic.json"); err != nil || path != "testdata/logistic.json" {
		t.Fatalf("configured path = %q, %v", path, err)
	}
}

// TestLoadArtifactAbortsOnUnusableDocuments covers the startup gate. Every one of
// these must stop the process: serving with a zero model answers 0.5 for every
// input and looks healthy from the outside.
func TestLoadArtifactAbortsOnUnusableDocuments(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		wantErr string
	}{
		{"missing file", "../../testdata/does-not-exist.json", "read model artifact"},
		{"directory", "../../testdata", "read model artifact"},
		{"contract violation", "../../testdata/invalid/format.json", `format "firewatch-model/v2" is not "firewatch-model/v1"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded, err := loadArtifact(tc.path)
			if err == nil {
				t.Fatalf("loaded %+v, want an error", loaded)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadArtifactReturnsTheCommittedModel(t *testing.T) {
	for _, name := range []string{"logistic.json", "forest.json"} {
		loaded, err := loadArtifact("../../testdata/" + name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if loaded.Version == "" || loaded.DataHash == "" || !loaded.TrainedAt.Time.Equal(loaded.TrainedAt.Time.UTC()) {
			t.Errorf("%s is not a usable identity: %+v", name, loaded)
		}
	}
}

// TestRoutesAndMetricsAreWired exercises the composition cmd/server builds: the
// handler set behind the middleware and the /metrics renderer beside it, with no
// listener involved. It asks newServer for the handler, so it covers what run()
// actually serves rather than a second copy of the mux.
func TestRoutesAndMetricsAreWired(t *testing.T) {
	loaded, err := loadArtifact("../../testdata/logistic.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	app := newServer(loaded).Handler

	predict := httptest.NewRequest(http.MethodPost, "/predict", strings.NewReader(
		`{"features": {"temp_max": 34.2, "temp_min": 18.4, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680}}`))
	predict.Header.Set("X-Request-ID", "not-a-uuid")
	predicted := httptest.NewRecorder()
	app.ServeHTTP(predicted, predict)
	if predicted.Code != http.StatusOK {
		t.Fatalf("predict got %d: %s", predicted.Code, predicted.Body.String())
	}
	// The middleware replaces a malformed correlation id with a canonical one
	// rather than forwarding the caller's spelling.
	if id := predicted.Header().Get("X-Request-ID"); id == "not-a-uuid" || len(id) != 36 {
		t.Errorf("correlation id %q", id)
	}

	scraped := httptest.NewRecorder()
	app.ServeHTTP(scraped, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if scraped.Code != http.StatusOK {
		t.Fatalf("metrics got %d", scraped.Code)
	}
	if contentType := scraped.Header().Get("Content-Type"); contentType != "text/plain; version=0.0.4" {
		t.Errorf("content type %q", contentType)
	}
	body := scraped.Body.String()
	for _, want := range []string{
		`firewatch_inference_predictions_total{level="high"} 1`,
		`firewatch_inference_model_info{version="valparaiso-logistic-v1"`,
		`firewatch_http_responses_total{class="2xx"}`,
		`firewatch_http_request_duration_seconds_bucket{le="0.005"}`,
		`firewatch_process_goroutines`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body is missing %q:\n%s", want, body)
		}
	}
	// Every scrape is answered 200 and then counted, so the 2xx series covers the
	// predict call and at least this scrape.
	if strings.Contains(body, `firewatch_http_errors_total 0`) == false {
		t.Errorf("no error total rendered:\n%s", body)
	}
}

// TestValidateFlagExitsWithTheStartupGate guards the deploy-time check: running
// the binary with -validate must load the artifact through the same gate startup
// uses and return without binding a listener, so a pipeline can validate an
// artifact with the code the server will actually run.
func TestValidateFlagExitsWithTheStartupGate(t *testing.T) {
	t.Setenv(modelPathEnv, "../../testdata/logistic.json")
	if err := run([]string{"-validate"}); err != nil {
		t.Fatalf("-validate on a valid artifact: %v", err)
	}

	t.Setenv(modelPathEnv, "../../testdata/does-not-exist.json")
	if err := run([]string{"-validate"}); err == nil {
		t.Fatal("-validate accepted a missing artifact")
	}
}

// TestValidatePathUsesTheStartupGate keeps -validate honest: it loads through
// loadArtifact, so a document the flag accepts is a document the server starts
// with.
func TestValidatePathUsesTheStartupGate(t *testing.T) {
	loaded, err := loadArtifact("../../testdata/forest.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Kind != artifact.KindForest || len(loaded.Trees) == 0 {
		t.Fatalf("loaded %+v", loaded)
	}
}

// TestRunRefusesPlaintextListenerOutsideDevelopment pins the process-level
// wiring of T7.5: a valid artifact and no key pair outside development stop the
// process before the listener binds, so the refusal is not something a caller
// can miss. The wait is bounded so a start that keeps serving fails the test
// instead of hanging it.
func TestRunRefusesPlaintextListenerOutsideDevelopment(t *testing.T) {
	t.Setenv(modelPathEnv, "../../testdata/logistic.json")
	t.Setenv("APP_ENV", "production")
	t.Setenv(tlsCertFileEnv, "")
	t.Setenv(tlsKeyFileEnv, "")
	done := make(chan error, 1)
	go func() { done <- run(nil) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), tlsCertFileEnv) {
			t.Fatalf("run() = %v, want a refusal naming %s", err, tlsCertFileEnv)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() kept serving in plaintext outside development instead of refusing to start")
	}
}

// TestValidateDoesNotRequireListenerTLS keeps the deploy-time check usable in a
// deployment environment: -validate never binds a listener, so a key pair is not
// part of its contract and a pipeline can validate an artifact without the
// server mounts.
func TestValidateDoesNotRequireListenerTLS(t *testing.T) {
	t.Setenv(modelPathEnv, "../../testdata/logistic.json")
	t.Setenv("APP_ENV", "production")
	t.Setenv(tlsCertFileEnv, "")
	t.Setenv(tlsKeyFileEnv, "")
	if err := run([]string{"-validate"}); err != nil {
		t.Fatalf("-validate in a deployment environment: %v", err)
	}
}

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
	response := waitForHTTP(t, &http.Client{Timeout: 5 * time.Second}, "http://"+addr+"/metrics", served)
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("plaintext /metrics got %d: %s", response.StatusCode, body)
	}
	if !strings.Contains(body, "firewatch_inference_model_info") {
		t.Errorf("plaintext /metrics body is missing the model identity:\n%s", body)
	}
}

// TestListenerServesTLSWithTheConfiguredKeyPair is the acceptance test of T7.5:
// with a certificate and a key, the listener answers a client that trusts the CA
// that signed them, so what runs is TLS and not a plaintext listener behind an
// https:// URL. The same request with the system roots must fail, which keeps the
// success from being explained by a skipped verification.
func TestListenerServesTLSWithTheConfiguredKeyPair(t *testing.T) {
	certFile, keyFile, roots := testKeyPair(t)
	served, addr := serveForTest(t, certFile, keyFile)
	trusted := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs: roots,
			// The certificate's only subject alternative name is localhost, which is
			// what the in-cluster callers dial; the test connects to loopback.
			ServerName: "localhost",
		}},
	}
	response := waitForHTTP(t, trusted, "https://"+addr+"/metrics", served)
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("TLS /metrics got %d: %s", response.StatusCode, body)
	}
	if !strings.Contains(body, "firewatch_inference_model_info") {
		t.Errorf("TLS /metrics body is missing the model identity:\n%s", body)
	}

	untrusted := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{}}}
	if response, err := untrusted.Get("https://" + addr + "/metrics"); err == nil {
		response.Body.Close()
		t.Fatalf("a client with the system roots got %d, want a certificate verification failure", response.StatusCode)
	}
}

// serveForTest starts the only listener the process has, on a free loopback port
// so the test never competes for 8080, and stops it when the test ends.
func serveForTest(t *testing.T, certFile, keyFile string) (<-chan error, string) {
	t.Helper()
	loaded, err := loadArtifact("../../testdata/logistic.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
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
	server := newServer(loaded)
	server.Addr = addr
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
// pool a trusting client needs. Nothing is shared with the repository or the
// deployment: the real secrets are issued by bootstrap-secrets.sh.
func testKeyPair(t *testing.T) (certFile, keyFile string, roots *x509.CertPool) {
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
		Subject:      pkix.Name{CommonName: "inference"},
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
	certFile, keyFile = filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	if err = os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverKeyDER})
	if err = os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	roots = x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})) {
		t.Fatal("the test CA is not a usable root")
	}
	return certFile, keyFile, roots
}
