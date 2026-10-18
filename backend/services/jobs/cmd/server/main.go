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

	"firewatch.local/jobs/internal/handler"
	"firewatch.local/jobs/internal/messaging"
	"firewatch.local/jobs/internal/metrics"
	"firewatch.local/jobs/internal/repository"
	"firewatch.local/jobs/migrations"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
		for _, migration := range []struct {
			version int
			sql     string
		}{
			{version: 1, sql: migrations.Initial},
			{version: 2, sql: migrations.Zones},
			{version: 3, sql: migrations.SyntheticScenario},
			{version: 4, sql: migrations.JobFailureState},
		} {
			var exists bool
			if err = tx.QueryRow(migrationCtx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, migration.version).Scan(&exists); err != nil {
				return err
			}
			if exists {
				continue
			}
			if _, err = tx.Exec(migrationCtx, migration.sql); err != nil {
				return fmt.Errorf("migration %d: %w", migration.version, err)
			}
			if _, err = tx.Exec(migrationCtx, `INSERT INTO schema_migrations(version) VALUES($1)`, migration.version); err != nil {
				return fmt.Errorf("migration %d: %w", migration.version, err)
			}
		}
		// The schema transaction must be committed before seeding: the seed opens
		// its own transaction, and the zones table created above would otherwise
		// still be invisible to it. Seeding runs on every migrate, not only when a
		// migration was pending, which is what makes a fresh volume settle into the
		// same state as an existing one. A failed seed returns non-zero so a broken
		// catalogue cannot ship silently.
		if err = tx.Commit(migrationCtx); err != nil {
			return err
		}
		seeded, err := repository.SeedZonesEmbedded(migrationCtx, db)
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
	// Validate both bounds before starting any broker worker.
	dlqReadyMaxDepth, err := parseDLQReadyMaxDepth(os.Getenv("DLQ_READY_MAX_DEPTH"))
	if err != nil {
		return err
	}
	dlqReadyMaxAge, err := parseDLQReadyMaxAge(os.Getenv("DLQ_READY_MAX_AGE"))
	if err != nil {
		return err
	}
	broker := &messaging.Broker{DB: db, URL: os.Getenv("AMQP_URL"), Handle: store.Complete, Fail: store.Fail}
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); broker.Run(ctx) }()
	defer func() { stop(); <-workerDone }()
	snapshot := func() messaging.DeadLetterSnapshot { return broker.Snapshot(time.Now(), dlqReadyMaxAge) }
	mux := http.NewServeMux()
	check := statusEvaluator(db.Ping, snapshot, dlqReadyMaxDepth, dlqReadyMaxAge)
	mux.Handle("/v1/", handler.New(store, verifier, check))
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.Handle("GET /health/ready", healthHandler(check))
	var requests atomic.Uint64
	// The HTTP layer and the process are this service's own signals; the broker and
	// dead-letter counters stay in messaging.MetricsText and are rendered first,
	// unchanged, so nothing that was already exported is lost.
	httpMetrics := &metrics.HTTPMetrics{}
	queuedAge := func(ctx context.Context) float64 { return queuedAgeSeconds(ctx, db) }
	mux.Handle("GET /metrics", metricsHandler(snapshot, &requests, broker, queuedAge, httpMetrics.Text, metrics.RuntimeText))
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
		started := time.Now()
		// One wrapper times and classifies every response of this service, including
		// its errors and /metrics itself, without a handler changing.
		recorder := &statusRecorder{ResponseWriter: w}
		mux.ServeHTTP(recorder, r.WithContext(c))
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		httpMetrics.Observe(status, time.Since(started))
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

func readinessHandler(ping func(context.Context) error, snapshot func() messaging.DeadLetterSnapshot, maxDepth int64) http.Handler {
	return healthHandler(statusEvaluator(ping, snapshot, maxDepth, 6*time.Second))
}

// healthHandler serves the Kubernetes readiness probe. S2: disposition to
// receive traffic is decided by the bounded database ping alone, so broker and
// dead-letter state can never change this status code. S3: the response body is
// the full operational status, shape-identical between the 200 and the 503
// answer, so a degraded broker or a dead-letter backlog stays visible in
// dependencies.broker and dead_letter even when the probe answers 200.
func healthHandler(check handler.StatusCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := check(r.Context())
		code := http.StatusServiceUnavailable
		if s.Dependencies.Database.State == "ready" {
			code = http.StatusOK
		}
		respondStatus(w, code, s)
	})
}

// respondStatus writes the documented status object with the same cache and
// content-type discipline as the handlers' respond helper.
func respondStatus(w http.ResponseWriter, status int, s handler.Status) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(s)
}

// metricsHandler renders the Prometheus text body of /metrics. It is a named seam
// so the surface can be exercised without run(). The delivery and dead-letter
// counters (messaging.MetricsText) are rendered first and unchanged - they are
// what the dead-letter and stuck-job alert rules are written against - and the
// additional blocks arrive as renderers, so this service can grow its own
// signals without widening the parameter list or touching the pinned delivery
// format.
//
// queuedAge is injected rather than read from a pool here for the same reason:
// the read is bounded and can fail, and that behaviour is tested on its own.
func metricsHandler(snapshot func() messaging.DeadLetterSnapshot, requests *atomic.Uint64, broker *messaging.Broker, queuedAge func(context.Context) float64, extra ...func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		s := snapshot()
		fmt.Fprint(w, messaging.MetricsText(requests.Load(), s.Ready, broker.Published.Load(), broker.Processed.Load(), broker.Rejected.Load(), s.Depth, queuedAge(r.Context()), s.AgeSeconds))
		for _, render := range extra {
			fmt.Fprint(w, render())
		}
	})
}

// statusRecorder captures the response status so the middleware can count
// responses by class and time the request without changing any handler. A
// handler that writes a body without calling WriteHeader reports 200, which is
// the code net/http would send.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

// metricsQuerier is the read seam /metrics uses to measure the oldest queued job
// without depending on the concrete pool. It exists so the age handling - a
// value, a SQL NULL, or a failed read - is testable without a database.
type metricsQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// queuedAgeSeconds reads how long the oldest queued job has waited, so a job that
// never receives an answer stops being invisible without a clock ever marking
// legitimate backlog as failed. The read is bounded: a short deadline and a
// single-row query, so a slow or unavailable database cannot stall /metrics. No
// queued job (SQL NULL) and a failed read both report QueuedAgeUnknown rather
// than a young, healthy-looking zero.
func queuedAgeSeconds(ctx context.Context, db metricsQuerier) float64 {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var waited *float64
	err := db.QueryRow(ctx, `SELECT EXTRACT(EPOCH FROM now()-requested_at)::double precision FROM jobs WHERE state='queued' ORDER BY requested_at LIMIT 1`).Scan(&waited)
	if err != nil || waited == nil {
		return messaging.QueuedAgeUnknown
	}
	return *waited
}
