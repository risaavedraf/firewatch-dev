package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"firewatch.local/prevention/internal/handler"
	"firewatch.local/prevention/internal/repository"
	"firewatch.local/prevention/migrations"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// listenAddr is the prevention service's own port, fixed by the frozen contract
// (odd/tasks/presentation-client-ready.md §«Contrato congelado de prevention»).
// It is deliberately not 8080: that port belongs to the gateway on the local
// stack, and a distinctive port makes it obvious which process answered.
const listenAddr = ":8084"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("service_stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("DATABASE_URL") == "" {
		return errors.New("DATABASE_URL required")
	}
	config, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return errors.New("database setup failed")
	}
	defer db.Close()
	if len(os.Args) == 2 && os.Args[1] == "migrate" {
		return migrate(ctx, db)
	}
	issuer, jwks := os.Getenv("OIDC_ISSUER"), os.Getenv("OIDC_JWKS_URL")
	for _, value := range []string{issuer, jwks} {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || (u.Scheme != "https" && !(os.Getenv("APP_ENV") == "development" && u.Scheme == "http")) {
			return errors.New("invalid OIDC URL; HTTPS required outside development")
		}
	}
	if os.Getenv("OIDC_AUDIENCE") == "" {
		return errors.New("OIDC_AUDIENCE required")
	}
	keyCtx := oidc.ClientContext(ctx, &http.Client{Timeout: 5 * time.Second})
	verifier := oidc.NewVerifier(issuer, oidc.NewRemoteKeySet(keyCtx, jwks), &oidc.Config{ClientID: os.Getenv("OIDC_AUDIENCE"), SupportedSigningAlgs: []string{"RS256"}})
	store := repository.Store{DB: db}
	mux := http.NewServeMux()
	mux.Handle("/v1/", handler.New(store, verifier))
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.Handle("GET /health/ready", readyHandler(db.Ping))
	outer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil {
			id = uuid.New()
		}
		w.Header().Set("X-Request-ID", id.String())
		r.Header.Set("X-Request-ID", id.String())
		c, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(c))
	})
	srv := &http.Server{Addr: listenAddr, Handler: outer, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "addr", listenAddr)
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// migrate applies the pending migrations of this service.
//
// The ledger is the shared schema_migrations table of the database the schema
// lives in, and the advisory lock is the same one the sibling services take, so
// two migration runners can never apply schemas concurrently. Applying this
// service's migration is a no-op on a database that already recorded it, and it
// is independent of whether the sibling migrations ran first: the ledger only
// says which versions are present, never in which order they arrived.
func migrate(ctx context.Context, db *pgxpool.Pool) error {
	migrationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := db.Begin(migrationCtx)
	if err != nil {
		return errors.New("migration connection failed")
	}
	defer tx.Rollback(migrationCtx)
	if _, err = tx.Exec(migrationCtx, `SELECT pg_advisory_xact_lock(1)`); err != nil {
		return err
	}
	if _, err = tx.Exec(migrationCtx, `CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY)`); err != nil {
		return err
	}
	rows, err := tx.Query(migrationCtx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	applied := map[int]bool{}
	for rows.Next() {
		var version int
		if err = rows.Scan(&version); err != nil {
			rows.Close()
			return err
		}
		applied[version] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, migration := range migrations.All() {
		if applied[migration.Version] {
			continue
		}
		if _, err = tx.Exec(migrationCtx, migration.SQL); err != nil {
			return fmt.Errorf("migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		if _, err = tx.Exec(migrationCtx, `INSERT INTO schema_migrations(version) VALUES($1)`, migration.Version); err != nil {
			return fmt.Errorf("migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		slog.Info("migration_applied", "version", migration.Version, "name", migration.Name)
	}
	return tx.Commit(migrationCtx)
}

// readiness is the body of the readiness probe. It carries only the dependency
// this service has - its own database. It deliberately does not reuse the jobs/risk
// operational status shape: those services also report a broker and a dead-letter
// queue, and reporting a broker this service never talks to would be a fabricated
// dependency.
type readiness struct {
	CheckedAt    time.Time                  `json:"checked_at"`
	Ready        bool                       `json:"ready"`
	Reason       string                     `json:"reason"`
	Dependencies map[string]readinessDepMap `json:"dependencies"`
}

// readinessDepMap is one dependency's state.
type readinessDepMap struct {
	State string `json:"state"`
}

// readyHandler answers the Kubernetes readiness probe from a bounded database
// ping alone. A failed ping is genuinely not ready, so it answers 503 with the
// same body shape, which keeps the probe answer and the reported reason from
// disagreeing.
func readyHandler(ping func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		status := readiness{
			CheckedAt:    time.Now().UTC(),
			Ready:        true,
			Dependencies: map[string]readinessDepMap{"database": {State: "ready"}},
		}
		code := http.StatusOK
		if ping(ctx) != nil {
			status.Ready = false
			status.Reason = "database_unavailable"
			status.Dependencies["database"] = readinessDepMap{State: "degraded"}
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(status)
	})
}
