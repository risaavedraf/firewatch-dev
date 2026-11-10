package classify

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestInferenceClassifierTrustsTheConfiguredCA is T7.3's outbound half for the
// model path, asserted through a real handshake instead of by inspecting the
// client: an inference stand-in serves its model document and its prediction over
// TLS with a chain rooted in a throwaway CA, so construction succeeds only if the
// configured pool answered for the certificate. Without the pool the same
// construction must fail certificate verification, which proves the pool is what
// makes the deployment work rather than the system roots.
func TestInferenceClassifierTrustsTheConfiguredCA(t *testing.T) {
	pool, certificate := testInferencePKI(t)

	mux := http.NewServeMux()
	mux.HandleFunc(modelDocumentPath, modelDocumentHandler(fixtureModelDocument))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, predictionBody("medium", 0.5))
	})
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)

	classifier, err := NewMLFixtureV1WithRootCAs(context.Background(), server.URL, time.Second, pool)
	if err != nil {
		t.Fatalf("the classifier refused a service rooted in the configured CA: %v", err)
	}
	assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t))
	if err != nil {
		t.Fatalf("classify over TLS with the configured CA: %v", err)
	}
	if assessment.Level != LevelMedio {
		t.Fatalf("level = %q, want the level the stand-in served", assessment.Level)
	}

	// Without TLS_CA_FILE the classifier keeps the bare client, so the same
	// construction cannot verify the chain and must stop startup by name.
	_, err = NewMLFixtureV1(context.Background(), server.URL, time.Second)
	var verification *tls.CertificateVerificationError
	if err == nil || !errors.As(err, &verification) {
		t.Fatalf("construction without the internal CA got %v, want a certificate verification failure", err)
	}
}

// testInferencePKI issues a throwaway CA and a server certificate for the loopback
// address in the test's own temp directory, then loads them back from PEM so the
// anchor and the served chain are files the way startup reads them. Nothing is
// shared with the repository or the deployment.
func testInferencePKI(t *testing.T) (*x509.CertPool, tls.Certificate) {
	t.Helper()
	dir := t.TempDir()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "firewatch-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "inference"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		// The httptest server is dialed by the loopback address in its URL, so that
		// exact IP is the only name the certificate needs.
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("server certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("server key encoding: %v", err)
	}
	certPath, keyPath := filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	if err = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), 0o600); err != nil {
		t.Fatalf("write server certificate: %v", err)
	}
	if err = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write server key: %v", err)
	}
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("load server key pair: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})) {
		t.Fatal("the test CA is not a usable root")
	}
	return pool, certificate
}
