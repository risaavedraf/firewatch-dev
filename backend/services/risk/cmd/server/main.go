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
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/handler"
	"firewatch.local/risk/internal/messaging"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/service"
	"firewatch.local/risk/internal/source"
	"firewatch.local/risk/internal/zones"
	"firewatch.local/risk/migrations"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
		applied := make(map[int]bool)
		rows, err := tx.Query(migrationCtx, `SELECT version FROM schema_migrations`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var version int
			if err = rows.Scan(&version); err != nil {
				rows.Close()
				return err
			}
			applied[version] = true
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return rowsErr
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
		}
		// The schema transaction must be committed before seeding: the seed opens
		// its own transaction, and the zones table created above would otherwise
		// still be invisible to it. Seeding runs on every migrate, not only when a
		// migration was pending, which is what makes a fresh volume settle into the
		// same state as an existing one.
		if err = tx.Commit(migrationCtx); err != nil {
			return err
		}
		seeded, err := zones.SeedEmbedded(migrationCtx, db)
		if err != nil {
			return fmt.Errorf("seed zones: %w", err)
		}
		slog.Info("zones_seeded", "count", seeded)
		return nil
	}
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("AMQP_URL") == "" {
		return errors.New("database and broker configuration required")
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
	// The active classifier is chosen by configuration rather than by code. The
	// default keeps the existing deployment starting without new configuration.
	// The selected version must have a row in the models table (migration 002),
	// because batches.model_version is a foreign key into it; a new model version
	// is therefore a new classifier version plus a new migration row.
	classifiers := classify.NewRegistry()
	if err := classifiers.Register(classify.NewAcademicRuleV1()); err != nil {
		return fmt.Errorf("register classifier: %w", err)
	}
	activeModel := os.Getenv("MODEL_ID")
	if activeModel == "" {
		activeModel = classify.AcademicRuleV1ID
	}
	active, err := classifiers.Select(activeModel)
	if err != nil {
		return fmt.Errorf("invalid MODEL_ID configuration %q: %w", activeModel, err)
	}
	// Fail closed before serving: the selected classifier version must exist in the
	// models registry, and when its fingerprint is frozen in code the stored
	// params_hash must match. This closes the otherwise silent failure this unit is
	// about: a classifier registered in Go but absent from - or divergent in - the
	// models table would start healthy and then discard every job, because
	// batches.model_version is a foreign key into that table.
	//
	// active is deliberately NOT required here: it expresses the registry's
	// lifecycle, while the selected version already comes from MODEL_ID. The
	// migrate path returns above, before this check, so creating the schema is not
	// blocked by the row this check reads.
	storedHash, registered, err := store.ModelParamsHash(ctx, activeModel)
	if err != nil {
		return fmt.Errorf("verify classifier %q against the models registry: %w", activeModel, err)
	}
	if err := classify.VerifyRegisteredModel(activeModel, registered, storedHash); err != nil {
		return err
	}
	// The fixture is the only provider that exists today because real ingestion
	// is deferred. Registering a real provider here is what flips the persisted
	// demo flag to false: that flag is derived from the provider's provenance, so
	// it is not hardcoded anywhere.
	provider := source.NewFixtureProvider()
	publisher := service.Publisher{Provider: provider, Classifier: active, Store: store}
	// Validate both bounds before starting any broker worker.
	dlqReadyMaxDepth, err := parseDLQReadyMaxDepth(os.Getenv("DLQ_READY_MAX_DEPTH"))
	if err != nil {
		return err
	}
	dlqReadyMaxAge, err := parseDLQReadyMaxAge(os.Getenv("DLQ_READY_MAX_AGE"))
	if err != nil {
		return err
	}
	broker := &messaging.Broker{DB: db, URL: os.Getenv("AMQP_URL"), Handle: publisher.Publish}
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); broker.Run(ctx) }()
	defer func() { stop(); <-workerDone }()
	snapshot := func() messaging.DeadLetterSnapshot { return broker.Snapshot(time.Now(), dlqReadyMaxAge) }
	mux := http.NewServeMux()
	check := statusEvaluator(db.Ping, snapshot, dlqReadyMaxDepth, dlqReadyMaxAge)
	mux.Handle("/v1/", handler.NewWithStatus(store, verifier, check, publisher.Provider))
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.Handle("GET /health/ready", healthHandler(check))
	var requests atomic.Uint64
	mux.Handle("GET /metrics", metricsHandler(snapshot, &requests, broker))
	outer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
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
	srv := &http.Server{Addr: ":8080", Handler: outer, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// defaultDLQReadyMaxDepth is the documented default for DLQ_READY_MAX_DEPTH: any
// dead letter means the consumer is discarding work, so readiness must degrade
// until an operator looks at it.
const defaultDLQReadyMaxDepth = 0

// parseDLQReadyMaxDepth reads the readiness threshold once at startup. An unset
// value uses the documented default; anything that is not a non-negative integer
// is a configuration error rather than a silent fallback.
func parseDLQReadyMaxDepth(raw string) (int64, error) {
	if raw == "" {
		return defaultDLQReadyMaxDepth, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid DLQ_READY_MAX_DEPTH %q: must be a non-negative integer", raw)
	}
	return value, nil
}

func parseDLQReadyMaxAge(raw string) (time.Duration, error) {
	if raw == "" {
		return 6 * time.Second, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid DLQ_READY_MAX_AGE %q: must be a positive duration", raw)
	}
	return value, nil
}

// statusEvaluator is the single evaluator shared by both routes. It measures a
// bounded database ping, one dead-letter snapshot taken immediately after it
// (including when the database failed), and builds the dependency report from
// both. Its Ready and Reason are always the full composite decision
// (messaging.ReadyReason), so the authenticated status route and /health/ready
// can never disagree about what they measured or about the decision itself.
// /health/ready derives its routing code from the database dependency alone in
// healthHandler, which keeps the body's ready field the composite decision (S2,
// S3).
func statusEvaluator(ping func(context.Context) error, snapshot func() messaging.DeadLetterSnapshot, maxDepth int64, maxAge time.Duration) handler.StatusCheck {
	return func(parent context.Context) handler.Status {
		ctx, cancel := context.WithTimeout(parent, time.Second)
		defer cancel()
		dbOK := ping(ctx) == nil
		// Snapshot exactly once after ping, including database failure.
		s := snapshot()
		ready, reason := messaging.ReadyReason(dbOK, s.Ready, s.Depth, maxDepth)
		ageLimit := maxAge.Seconds()
		d := handler.DeadLetterStatus{State: "unknown", MaxDepth: &maxDepth, MaxAgeSeconds: &ageLimit}
		if s.Depth >= 0 {
			depth := s.Depth
			d.Depth = &depth
			d.State = "ready"
			if depth > maxDepth {
				d.State = "degraded"
			}
		}
		if !s.LastSuccess.IsZero() && s.AgeSeconds >= 0 {
			sample := s.LastSuccess.UTC()
			d.SampledAt = &sample
			d.AgeSeconds = &s.AgeSeconds
		}
		database, broker := "degraded", "degraded"
		if dbOK {
			database = "ready"
		}
		if s.Ready {
			broker = "ready"
		}
		return handler.Status{CheckedAt: time.Now().UTC(), Ready: ready, Reason: reason, Dependencies: handler.Dependencies{Database: handler.Dependency{State: database}, Broker: handler.Dependency{State: broker}, DeadLetter: d}}
	}
}

// metricsHandler renders the Prometheus text body of /metrics. It is a named
// seam so the metrics surface can be exercised without run(): readiness no longer
// depends on the broker, and S3 requires the broker readiness and the
// dead-letter depth and age to stay visible here, where an operator can still see
// them. It reads the broker readiness from the dead-letter snapshot, exactly as
// the inline handler did before.
func metricsHandler(snapshot func() messaging.DeadLetterSnapshot, requests *atomic.Uint64, broker *messaging.Broker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		s := snapshot()
		fmt.Fprint(w, messaging.MetricsText(requests.Load(), s.Ready, broker.Published.Load(), broker.Processed.Load(), broker.Rejected.Load(), s.Depth, s.AgeSeconds))
	})
}

// healthHandler serves /health/ready. The routing decision is the bounded
// database ping alone (S2), so broker connectivity and a dead-letter backlog can
// never change this status code. The body is the full operational status the
// authenticated status route returns (S3): its ready field is always the
// composite dependency decision and may therefore be false while the probe
// answers 200, and a 503 still carries the non-empty reason the same evaluator
// produced.
func healthHandler(check handler.StatusCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := check(r.Context())
		code := http.StatusServiceUnavailable
		if s.Dependencies.Database.State == "ready" {
			code = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(s)
	})
}
