package handler

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// RootCAsFromFile loads the PEM bundle TLS_CA_FILE names into the pool the
// gateway's outbound clients trust. An empty path returns a nil pool, which is
// what keeps the system roots and the behaviour this gateway had before the
// internal CA existed. An unreadable or certificate-free bundle is refused
// instead: falling back to the system roots would turn a missing mount into a
// handshake failure far from the cause, and the deployment only ever talks to
// services this CA vouches for.
func RootCAsFromFile(path string) (*x509.CertPool, error) {
	if path == "" {
		return nil, nil
	}
	bundle, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bundle) {
		return nil, fmt.Errorf("%s holds no PEM certificate", path)
	}
	return pool, nil
}

// newUpstreamTransport builds the transport each upstream proxy owns. A nil
// pool leaves TLSClientConfig unset on purpose: that is byte for byte the
// transport this gateway used before TLS_CA_FILE existed, so an unset variable
// cannot reshape the connection pool.
func newUpstreamTransport(rootCAs *x509.CertPool) *http.Transport {
	transport := &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 20, IdleConnTimeout: 60 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
	if rootCAs != nil {
		transport.TLSClientConfig = &tls.Config{RootCAs: rootCAs}
	}
	return transport
}
