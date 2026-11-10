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
