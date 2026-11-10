package main

import (
	"errors"

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
