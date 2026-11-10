package main

import (
	"strings"
	"testing"
)

const (
	// The three messages the operator can read. Naming the variable is the point:
	// an operator with a half-mounted secret must not have to guess which half.
	certMissingOutsideDevelopment = "TLS_CERT_FILE is required outside development: set TLS_CERT_FILE and TLS_KEY_FILE, or run with APP_ENV=development for plaintext"
	certMissingForConfiguredKey   = "TLS_CERT_FILE is required when TLS_KEY_FILE is set"
	keyMissingForConfiguredCert   = "TLS_KEY_FILE is required when TLS_CERT_FILE is set"
	// Any path works here: the guard decides from the strings alone, and the
	// listener is the only place that opens the files.
	certPath = "/etc/tls/tls.crt"
	keyPath  = "/etc/tls/tls.key"
)

// TestRequireListenerTLS pins the crossing contract of T7.5: a configured key
// pair serves TLS; no key pair keeps the plaintext of development and nothing
// else; every other combination refuses to start and names the variable that is
// missing. An empty want means the guard must accept.
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
		// The development exception is the literal value the local stack sets and
		// only with neither half configured. An unset or differently spelled
		// APP_ENV describes a deployment, and a deployment must not serve plaintext.
		{name: "no key pair in development", environment: "development"},
		{name: "no key pair outside development", environment: "staging", want: certMissingOutsideDevelopment},
		{name: "no key pair with an unset environment", want: certMissingOutsideDevelopment},
		{name: "the development exception is not case-insensitive", environment: "Development", want: certMissingOutsideDevelopment},
		{name: "the development exception does not apply to production", environment: "production", want: certMissingOutsideDevelopment},
		// A half-configured pair is not a configuration at all: it must be refused
		// even in development, and the message must name the half that is absent.
		{name: "certificate without a key outside development", environment: "staging", certFile: certPath, want: keyMissingForConfiguredCert},
		{name: "key without a certificate outside development", environment: "staging", keyFile: keyPath, want: certMissingForConfiguredKey},
		{name: "certificate without a key in development", environment: "development", certFile: certPath, want: keyMissingForConfiguredCert},
		{name: "key without a certificate in development", environment: "development", keyFile: keyPath, want: certMissingForConfiguredKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := requireListenerTLS(tc.environment, tc.certFile, tc.keyFile)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("the guard refused a configuration it must accept: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("the guard accepted a configuration it must refuse, want %q", tc.want)
			}
			if err.Error() != tc.want {
				t.Fatalf("the guard refused with %q, want %q", err.Error(), tc.want)
			}
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
	cert, key, err = startupListenerTLS("development")
	if err != nil || cert != "" || key != "" {
		t.Fatalf("development without TLS = %q, %q, %v", cert, key, err)
	}
}
