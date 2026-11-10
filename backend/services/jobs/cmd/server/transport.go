package main

import (
	"errors"
	"net/url"
	"os"

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
	// pgx leaves TLSConfig nil for every mode that negotiates no TLS, sslmode=disable
	// among them, so this is the condition rather than a string match on the URL.
	if cfg.ConnConfig.TLSConfig == nil {
		return errors.New("DATABASE_URL must require TLS outside development")
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
