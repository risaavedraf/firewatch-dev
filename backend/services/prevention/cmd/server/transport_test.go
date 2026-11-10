package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	plaintextDatabaseURL = "postgres://user:pass@db:5432/x?sslmode=disable"
	// verify-full is the only mode that both encrypts and authenticates the server,
	// so it is the only URL this test may treat as acceptable outside development.
	encryptedDatabaseURL       = "postgres://user:pass@db:5432/x?sslmode=verify-full"
	unauthenticatedDatabaseURL = "postgres://user:pass@db:5432/x?sslmode=require"
	fallbackDatabaseURL        = "postgres://user:pass@db:5432/x?sslmode=prefer"
	tlsGuardMessage            = "DATABASE_URL must require TLS outside development"
	tlsFallbackMessage         = "DATABASE_URL must not fall back to a plaintext connection outside development"
	tlsVerifyMessage           = "DATABASE_URL must verify the server certificate outside development"
)

// databaseConfigFor builds the pool configuration from a URL exactly as startup
// does, so the guard is exercised against what pgx derives from sslmode instead
// of a hand-made struct that proves nothing about the parsed value.
func databaseConfigFor(t *testing.T, databaseURL string) *pgxpool.Config {
	t.Helper()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse %q: %v", databaseURL, err)
	}
	return config
}

// assertGuardResult states the observable contract of the guard: either it
// accepts the configuration or it refuses it with exactly the message the
// operator will read. An empty want means the guard must accept.
func assertGuardResult(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("the guard refused a configuration it must accept: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("the guard accepted a configuration it must refuse, want %q", want)
	}
	if err.Error() != want {
		t.Fatalf("the guard refused with %q, want %q", err.Error(), want)
	}
}

// TestRequireTLSOutsideDevelopment pins the database rule: a URL that negotiates
// no TLS is a plaintext connection, and the only exception is the literal
// environment value "development". An unset or differently spelled value has to
// fail closed, because that spelling is what a deployment gets wrong.
func TestRequireTLSOutsideDevelopment(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment string
		databaseURL string
		want        string
	}{
		{name: "plaintext outside development", environment: "production", databaseURL: plaintextDatabaseURL, want: tlsGuardMessage},
		{name: "verified tls outside development", environment: "production", databaseURL: encryptedDatabaseURL},
		// Encrypted but unauthenticated, and encrypted with a way out: both are
		// refusals, because either one lets an active attacker succeed.
		{name: "require without verification outside development", environment: "production", databaseURL: unauthenticatedDatabaseURL, want: tlsVerifyMessage},
		{name: "prefer with a plaintext fallback outside development", environment: "production", databaseURL: fallbackDatabaseURL, want: tlsFallbackMessage},
		{name: "plaintext in development", environment: "development", databaseURL: plaintextDatabaseURL},
		{name: "plaintext with an unset environment", environment: "", databaseURL: plaintextDatabaseURL, want: tlsGuardMessage},
		{name: "the development exception is not case-insensitive", environment: "Development", databaseURL: plaintextDatabaseURL, want: tlsGuardMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGuardResult(t, requireTLSOutsideDevelopment(databaseConfigFor(t, tc.databaseURL), tc.environment), tc.want)
		})
	}
}

// TestRunRefusesPlaintextDatabaseOutsideDevelopment pins the wiring, not only
// the guard: the process stops on DATABASE_URL before it opens the pool, so a
// production start cannot reach a connection attempt. The address is a closed
// loopback port, so even a run that got as far as the pool fails immediately
// instead of reaching for a database that might exist.
func TestRunRefusesPlaintextDatabaseOutsideDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://user:pass@127.0.0.1:1/x?sslmode=disable")
	assertGuardResult(t, run(), tlsGuardMessage)
}
