package messaging

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
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeBrokerTimeout bounds every wait in the test below. The healthy path
// finishes in milliseconds; the bound exists so a regression that never dials,
// or a dial that never returns, fails the test instead of hanging the suite.
const fakeBrokerTimeout = 5 * time.Second

// testBrokerPKI issues a throwaway CA and a server certificate for host under the
// test's own temp directory and loads them back from PEM, so the trust anchor and
// the served chain are files the way startup reads them. Nothing is shared with
// the repository or the deployment: the real CA is issued by
// bootstrap-secrets.sh and never lands in the tree.
func testBrokerPKI(t *testing.T, host string) (*x509.CertPool, tls.Certificate) {
	t.Helper()
	ip := net.ParseIP(host)
	if ip == nil {
		t.Fatalf("host %q must be an IP literal so the test never depends on name resolution", host)
	}
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
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err = os.WriteFile(filepath.Join(dir, "ca.crt"), caPEM, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		// The certificate names the exact host the URL dials, so the ServerName
		// amqpConfig derives from that URL is what the chain is checked against.
		IPAddresses: []net.IP{ip},
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
	// The pool is built from the PEM the server chain was signed with, the same
	// way startup builds it from TLS_CA_FILE, so anchor and chain cannot silently
	// disagree.
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("the test CA is not a usable root")
	}
	return pool, certificate
}

// fakeTLSBroker is the smallest server that can answer the question this test
// asks: it terminates TLS with a certificate from the test CA and implements no
// AMQP at all. It reports exactly one handshake outcome, so the assertion is
// about the certificate chain and never about a number of retries.
type fakeTLSBroker struct {
	addr       string
	handshakes chan error
}

func newFakeTLSBroker(t *testing.T, certificate tls.Certificate) *fakeTLSBroker {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	broker := &fakeTLSBroker{addr: listener.Addr().String(), handshakes: make(chan error, 1)}
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			broker.handshakes <- err
			return
		}
		defer conn.Close()
		secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
		_ = secure.SetDeadline(time.Now().Add(fakeBrokerTimeout))
		handshakeErr := secure.Handshake()
		broker.handshakes <- handshakeErr
		if handshakeErr != nil {
			return
		}
		// Deliberately no Connection.Start: closing here fails the client's open
		// immediately on the missing frame instead of waiting out the dial
		// deadline. The handshake above is the whole event under assertion.
	}()
	return broker
}

// dialSession runs the broker's real connection path - the same session call the
// worker makes on every reconnect, not amqpConfig on its own - and returns both
// outcomes: what the fake broker saw during the TLS handshake and what the client
// reported.
func (p *fakeTLSBroker) dialSession(t *testing.T, broker *Broker) (serverErr, sessionErr error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), fakeBrokerTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- broker.session(ctx) }()
	select {
	case serverErr = <-p.handshakes:
	case <-ctx.Done():
		t.Fatal("the fake broker never reached a TLS handshake with the client")
	}
	select {
	case sessionErr = <-done:
	case <-ctx.Done():
		t.Fatal("session did not return after the fake broker closed the connection")
	}
	return serverErr, sessionErr
}

// TestBrokerSessionHandshakesOnlyWithTheConfiguredCA pins T7.2's outbound AMQP
// wiring where the tests that inspect dialConfig cannot: through the real
// connection path, with a TLS handshake that either happens or does not. A broker
// built with the internal CA must complete the handshake against a chain that
// roots in that CA and matches the host the URL names; a broker built without it
// must not complete the handshake at all, and must report a certificate
// verification failure instead of hanging or being refused.
//
// This is the coverage the call site lacked: a session that dialed with a
// configuration built from the URL alone would leave every other test in this
// package green, because they all inspect dialConfig in isolation.
func TestBrokerSessionHandshakesOnlyWithTheConfiguredCA(t *testing.T) {
	// 127.0.0.1 keeps name resolution out of the test: the certificate names that
	// exact IP, so the only variable left in the handshake is the root pool.
	const host = "127.0.0.1"
	pool, certificate := testBrokerPKI(t, host)
	for _, tc := range []struct {
		name          string
		rootCAs       *x509.CertPool
		wantHandshake bool
	}{
		{name: "the configured internal CA completes the handshake", rootCAs: pool, wantHandshake: true},
		{name: "no CA configured never completes the handshake", rootCAs: nil, wantHandshake: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broker := newFakeTLSBroker(t, certificate)
			serverErr, sessionErr := broker.dialSession(t, &Broker{
				URL:     "amqps://guest:guest@" + broker.addr + "/firewatch",
				RootCAs: tc.rootCAs,
			})
			// The fake broker speaks no AMQP, so the dial can never succeed: the
			// only question is where it failed.
			if sessionErr == nil {
				t.Fatal("session returned no error, but the fake broker serves no AMQP topology")
			}
			var verification *tls.CertificateVerificationError
			if !tc.wantHandshake {
				if serverErr == nil {
					t.Fatal("the fake broker completed a TLS handshake with no CA configured")
				}
				if !errors.As(sessionErr, &verification) {
					t.Fatalf("session failed with %v, want a certificate verification failure", sessionErr)
				}
				return
			}
			if serverErr != nil {
				t.Fatalf("the fake broker rejected the handshake with the configured CA: %v (client error: %v)", serverErr, sessionErr)
			}
			if errors.As(sessionErr, &verification) {
				t.Fatalf("the client rejected the certificate it was configured to trust: %v", sessionErr)
			}
		})
	}
}
