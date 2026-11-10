package main

import (
	"fmt"
	"net/http"
	"os"
)

// The listener's crossing contract is the one every service of the deployment
// shares (odd/tasks/kubernetes-deployment.md, L10): a PEM key pair in these two
// variables turns the listener into TLS, and outside the development exception
// plaintext is refused. There is no shared package on purpose; the repository
// duplicates this guard per service so each binary owns its own transport
// decision.
const (
	tlsCertFileEnv = "TLS_CERT_FILE"
	tlsKeyFileEnv  = "TLS_KEY_FILE"
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

// startupListenerTLS reads the two variables and returns the key pair the
// listener must serve. It is the seam run() calls, so the decision is testable
// without binding a port or mounting a certificate.
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
