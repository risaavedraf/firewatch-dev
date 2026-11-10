package handler

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// caBundleFromServer writes the certificate a test server presents as the PEM
// bundle TLS_CA_FILE names, inside a fresh temp dir. Writing the file rather
// than handing the certificate over in memory keeps these tests on the path a
// deployment walks: a PEM on disk, loaded and installed in the upstream pool.
func caBundleFromServer(t *testing.T, server *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	bundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(path, bundle, 0o600); err != nil {
		t.Fatalf("write the CA bundle: %v", err)
	}
	return path
}

// TestUpstreamTransportWithoutRootCAsIsUnchanged pins the no-CA case: the proxy
// keeps the system roots and the exact transport this gateway already had, so
// an unset TLS_CA_FILE cannot reshape the connection pool by accident.
func TestUpstreamTransportWithoutRootCAsIsUnchanged(t *testing.T) {
	transport := newUpstreamTransport(nil)
	if transport.TLSClientConfig != nil {
		t.Fatalf("the proxy gained a TLS client config without a CA: %+v", transport.TLSClientConfig)
	}
	if transport.MaxIdleConns != 100 || transport.MaxIdleConnsPerHost != 20 ||
		transport.IdleConnTimeout != 60*time.Second || transport.ResponseHeaderTimeout != 10*time.Second {
		t.Fatalf("the proxy transport changed beyond the CA: %+v", transport)
	}
}

// TestUpstreamTransportUsesTheConfiguredRootCAs pins the shape: the pool the
// startup loaded is the RootCAs of the transport every upstream proxy uses.
func TestUpstreamTransportUsesTheConfiguredRootCAs(t *testing.T) {
	rootCAs := x509.NewCertPool()
	transport := newUpstreamTransport(rootCAs)
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs != rootCAs {
		t.Fatalf("the transport does not carry the configured pool: %+v", transport.TLSClientConfig)
	}
}

// TestGatewayReachesUpstreamSignedByConfiguredCA is the negotiation, not the
// field: an https upstream whose certificate only the configured CA vouches for
// must be reachable, and the same upstream must be unreachable with the system
// roots. The second half is what proves the first half is not accidentally
// trusting everything.
func TestGatewayReachesUpstreamSignedByConfiguredCA(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()
	rootCAs, err := RootCAsFromFile(caBundleFromServer(t, upstream))
	if err != nil {
		t.Fatalf("load the CA bundle: %v", err)
	}
	target, _ := url.Parse(upstream.URL)
	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		r.Header.Set("Authorization", "Bearer valid")
		return r
	}
	if got := serve(NewWithRootCAs(verifier{}, target, target, target, "", rootCAs), request()); got != http.StatusAccepted {
		t.Fatalf("proxying to a CA-signed upstream got %d, want %d: the proxy is not using the configured pool", got, http.StatusAccepted)
	}
	if got := serve(New(verifier{}, target, target, target, ""), request()); got != http.StatusBadGateway {
		t.Fatalf("proxying to a CA-signed upstream with the system roots got %d, want %d: the CA is not what made the hop work", got, http.StatusBadGateway)
	}
}

// TestRootCAsFromFile pins the loader's three outcomes: no path keeps the
// system roots, a bundle becomes a pool, and an unusable bundle is refused
// instead of silently falling back to the system roots.
func TestRootCAsFromFile(t *testing.T) {
	if pool, err := RootCAsFromFile(""); pool != nil || err != nil {
		t.Fatalf("no path = %v, %v; want the system roots", pool, err)
	}
	garbage := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(garbage, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write the unusable bundle: %v", err)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "absent.crt"), garbage} {
		if pool, err := RootCAsFromFile(path); err == nil || pool != nil {
			t.Fatalf("%s = %v, %v; want a refusal", path, pool, err)
		}
	}
}
