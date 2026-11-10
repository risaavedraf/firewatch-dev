package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// requireTLSOutsideDevelopment refuses a database URL that does not negotiate
// TLS when the process is not running in development. A plaintext connection to
// the database inside the deployment network is still a plaintext connection:
// credentials, readings and the migration ledger all cross the wire in clear.
func requireTLSOutsideDevelopment(cfg *pgxpool.Config, environment string) error {
	// The exception is the literal value the local stack sets, matched the same way
	// the OIDC URL checks match it: an unset, empty or differently spelled APP_ENV
	// describes a deployment, and a deployment must not run unencrypted.
	if environment == "development" {
		return nil
	}
	// sslmode=prefer and sslmode=allow keep a fallback that negotiates no TLS, so a
	// non-nil TLSConfig proves nothing on its own: the connection may downgrade to
	// plaintext as soon as the TLS attempt fails. An attacker able to refuse the
	// handshake turns "encrypted by default" into "encrypted if convenient".
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if fallback.TLSConfig == nil {
			return errors.New("DATABASE_URL must not fall back to a plaintext connection outside development")
		}
	}
	if cfg.ConnConfig.TLSConfig == nil {
		return errors.New("DATABASE_URL must require TLS outside development")
	}
	// sslmode=require and sslmode=verify-ca encrypt without proving who answered:
	// an active attacker presents any certificate and the client accepts it. Only
	// verify-full both encrypts and authenticates, so only verify-full passes.
	if cfg.ConnConfig.TLSConfig.InsecureSkipVerify {
		return errors.New("DATABASE_URL must verify the server certificate outside development")
	}
	return nil
}

// requireSecureBrokerURL refuses a plaintext broker outside development, and
// refuses an explicit "do not verify" switch there as well: amqps with
// verification disabled protects nothing against an active attacker, and it is
// the setting someone reaches for when a certificate check fails.
func requireSecureBrokerURL(raw, environment string) error {
	if environment == "development" {
		return nil
	}
	parsed, err := url.Parse(raw)
	// An unparsable value cannot be amqps, so it is refused with the same message:
	// the operator needs the requirement, not the parser's complaint. Where the
	// URL's shape is validated already stays untouched; this only keeps the guard
	// safe on its own.
	if err != nil || parsed.Scheme != "amqps" {
		return errors.New("AMQP_URL must use amqps outside development")
	}
	if parsed.Query().Get("verify") == "verify_none" {
		return errors.New("AMQP_URL must not disable certificate verification outside development")
	}
	return nil
}

// startupBrokerURL reads AMQP_URL and refuses a plaintext broker before the
// worker is built. It is the seam the serving path uses, so the refusal is
// testable without a database or a broker, and it sits after the migrate branch:
// a migration job holds only DATABASE_URL and never dials the broker.
func startupBrokerURL(environment string) (string, error) {
	raw := os.Getenv("AMQP_URL")
	if err := requireSecureBrokerURL(raw, environment); err != nil {
		return "", err
	}
	return raw, nil
}

// The crossing contract fixes how this service is reached and what it trusts
// (odd/tasks/kubernetes-deployment.md, L10): a PEM key pair in these two variables
// turns the listener into TLS, and TLS_CA_FILE is the internal CA every client this
// process opens verifies with. There is no shared package on purpose; the
// repository duplicates these guards per service so each binary owns its own
// transport decision.
const (
	tlsCertFileEnv = "TLS_CERT_FILE"
	tlsKeyFileEnv  = "TLS_KEY_FILE"
	tlsCAFileEnv   = "TLS_CA_FILE"
)

// requireListenerTLS decides how the listener is served. Both halves present is
// TLS. Neither half present is the plaintext of development and nothing else: an
// unset or differently spelled APP_ENV describes a deployment, and a deployment
// must not serve in the clear. A half-configured pair is refused even in
// development, because a mount that lost one half is a configuration error, not a
// reason to fall back to plaintext.
func requireListenerTLS(environment, certFile, keyFile string) error {
	if certFile != "" && keyFile != "" {
		return nil
	}
	// The exception is the literal value the local stack sets, matched the same way
	// the OIDC and database guards match it.
	if certFile == "" && keyFile == "" && environment == "development" {
		return nil
	}
	// The operator needs the name of the missing half, so a pair with neither half
	// names both instead of only the first one checked.
	if certFile == "" && keyFile == "" {
		return fmt.Errorf(
			"%s is required outside development: set %s and %s, or run with APP_ENV=development for plaintext",
			tlsCertFileEnv, tlsCertFileEnv, tlsKeyFileEnv,
		)
	}
	if certFile == "" {
		return fmt.Errorf("%s is required when %s is set", tlsCertFileEnv, tlsKeyFileEnv)
	}
	return fmt.Errorf("%s is required when %s is set", tlsKeyFileEnv, tlsCertFileEnv)
}

// startupListenerTLS reads the two variables and returns the key pair the listener
// must serve. It is the seam run() calls, so the decision is testable without
// binding a port or mounting a certificate.
func startupListenerTLS(environment string) (certFile, keyFile string, err error) {
	certFile, keyFile = os.Getenv(tlsCertFileEnv), os.Getenv(tlsKeyFileEnv)
	if err := requireListenerTLS(environment, certFile, keyFile); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}

// listenAndServe binds the only listener this service has. A configured pair is
// served with ListenAndServeTLS; the plaintext branch is reachable only after
// requireListenerTLS accepted the development exception, because the key pair it
// returns is the one this function receives.
func listenAndServe(server *http.Server, certFile, keyFile string) error {
	if certFile != "" {
		return server.ListenAndServeTLS(certFile, keyFile)
	}
	return server.ListenAndServe()
}

// startupRootCAs reads TLS_CA_FILE and loads the internal CA into the pool the
// outbound clients of this service verify with. No variable is no pool, which keeps
// the system roots and the behaviour this process had before the internal CA
// existed; a variable that cannot be loaded stops startup and names itself, so a
// deployment never runs with trust it did not ask for.
func startupRootCAs() (*x509.CertPool, error) {
	path := os.Getenv(tlsCAFileEnv)
	if path == "" {
		return nil, nil
	}
	pool, err := rootCAsFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tlsCAFileEnv, err)
	}
	return pool, nil
}

// rootCAsFromFile loads a PEM bundle into a new pool. An unreadable or
// certificate-free bundle is refused instead of silently becoming the system roots:
// the deployment only ever talks to services this CA vouches for, so a missing
// mount has to be visible at startup and not as a handshake failure later.
func rootCAsFromFile(path string) (*x509.CertPool, error) {
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

// newJWKSClient is the client the OIDC key-set fetches use. A nil pool keeps the
// bare client with the system roots that this process used before TLS_CA_FILE
// existed; a configured pool makes that pool the only trust anchor the identity
// provider is reached with.
func newJWKSClient(rootCAs *x509.CertPool) *http.Client {
	client := &http.Client{Timeout: 5 * time.Second}
	if rootCAs != nil {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootCAs}}
	}
	return client
}
