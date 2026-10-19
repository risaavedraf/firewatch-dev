// Package cycle drives one automatic ingestion cycle: for every configured zone
// it asks the publication path to fetch, validate, classify and persist one
// assessment, and it records the freshness and state of the source so the API can
// report them.
//
// It owns scheduling and status, not data acquisition or classification. The
// publication seam keeps those contracts behind one interface, so replacing the
// fixture with a real source changes neither the cycle nor the API.
package cycle

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	observations "firewatch.local/contracts/observations/v1"
	"github.com/google/uuid"
)

// DefaultInterval is the automatic cycle period required by the bases: every 15
// minutes the system ingests, validates, classifies and publishes.
const DefaultInterval = 15 * time.Minute

// Publisher is the publication seam. Its single method runs the whole
// ingest-to-persist path for one zone as an INTERNAL, automatic publication: the
// batch is persisted but no job completion is emitted, because an automatic
// cycle is not a job and Jobs never stored the id the cycle mints.
//
// The seam is a method of its own instead of a flag on the explicit job
// publication, so the cycle cannot accidentally settle a job and the decision is
// never derived from the actor string: a request whose ActorID happens to read
// "cycle-scheduler" still settles its job when it arrives as a real delivery,
// and a scheduled publication never does.
type Publisher interface {
	PublishScheduled(ctx context.Context, request v1.Event) error
}

// DescriptorProvider is the read-only description of the active source, used to
// label the status and derive the demo flag of a cycle request.
type DescriptorProvider interface {
	Describe() observations.SourceDescriptor
}

// Options configures a Runner. Every field is optional and has a documented
// default, so production starts without configuration and a test pins the clock
// and the identifier generator.
type Options struct {
	// Zones are the zones one cycle publishes. Only zones present in the seeded
	// catalogue are acceptable to the store's foreign key.
	Zones []string
	// Interval is the period between automatic runs. It defaults to
	// DefaultInterval (15 minutes).
	Interval time.Duration
	// Now is the clock. It defaults to time.Now and must never be read by a test
	// that wants a reproducible run without one.
	Now func() time.Time
	// NewID mints the event, trace and job identifiers of one cycle request. It
	// defaults to UUID generation.
	NewID func() string
}

// ZoneResult is the outcome of one zone inside a cycle.
type ZoneResult struct {
	ZoneID    string `json:"zone_id"`
	Published bool   `json:"published"`
	Error     string `json:"error,omitempty"`
}

// RunResult is the outcome of one full cycle.
type RunResult struct {
	SourceID   string       `json:"source_id"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Complete   bool         `json:"complete"`
	Zones      []ZoneResult `json:"zones"`
}

// SourceStatus is the freshness and state of one source, as served by the API.
//
// LastRunAt is the last attempt (a failed attempt is still an execution), and
// Complete is whether that attempt published every zone. AgeSeconds is the age
// of the last attempt, so the UI can compare it against the 15 minute target.
//
// NextRunAt is the renewal deadline —the last attempt plus the configured
// interval— and NOT the scheduler's next tick: a manual run recomputes it while
// the automatic ticker keeps its own phase, so the two can legitimately differ.
// The API says the same thing, and the cycle panel labels it accordingly.
type SourceStatus struct {
	SourceID   string     `json:"source_id"`
	LastRunAt  *time.Time `json:"last_run_at"`
	NextRunAt  *time.Time `json:"next_run_at"`
	Complete   bool       `json:"complete"`
	AgeSeconds *float64   `json:"age_seconds"`
	LastError  string     `json:"last_error,omitempty"`
}

// Runner runs cycles on demand and on the 15 minute schedule, and keeps the
// state Statuses reports.
type Runner struct {
	source    DescriptorProvider
	publisher Publisher
	zones     []string
	interval  time.Duration
	now       func() time.Time
	newID     func() string

	mu        sync.Mutex
	lastRunAt *time.Time
	nextRunAt *time.Time
	complete  bool
	lastError string
}

// New returns a runner. A nil source or publisher is accepted so a misconfigured
// process fails on the first run with an attributable error instead of at
// startup; RunOnce refuses to act in that case.
func New(source DescriptorProvider, publisher Publisher, options Options) *Runner {
	runner := &Runner{
		source:    source,
		publisher: publisher,
		zones:     append([]string(nil), options.Zones...),
		interval:  options.Interval,
		now:       options.Now,
		newID:     options.NewID,
	}
	if runner.interval <= 0 {
		runner.interval = DefaultInterval
	}
	if runner.now == nil {
		runner.now = time.Now
	}
	if runner.newID == nil {
		runner.newID = uuid.NewString
	}
	return runner
}

// id returns the source identifier, or a stable placeholder when no source is
// configured so status stays attributable.
func (r *Runner) id() string {
	if r.source == nil {
		return "unconfigured"
	}
	if descriptor := r.source.Describe(); descriptor.ID != "" {
		return descriptor.ID
	}
	return "unconfigured"
}

// RunOnce executes one full cycle synchronously: each configured zone is
// published in order through the scheduled seam, a zone failure is recorded
// without aborting the rest, and the resulting state is kept for Statuses. Every
// zone's batch is persisted, and no zone reports a job completion, because no
// job row exists for an automatic cycle. Running it is idempotent with respect to
// its inputs: the clock and identifier generator decide every timestamp and id.
func (r *Runner) RunOnce(ctx context.Context) RunResult {
	result := RunResult{
		SourceID:  r.id(),
		StartedAt: r.now().UTC(),
		Complete:  true,
		Zones:     make([]ZoneResult, 0, len(r.zones)),
	}
	if r.publisher == nil {
		result.Complete = false
		result.Zones = append(result.Zones, ZoneResult{ZoneID: "", Error: "cycle publisher is not configured"})
		result.FinishedAt = r.now().UTC()
		r.record(result)
		return result
	}
	synthetic := false
	if r.source != nil {
		synthetic = r.source.Describe().Synthetic
	}
	for _, zone := range r.zones {
		request := v1.Event{
			ID:          r.newID(),
			Type:        v1.JobRequestedType,
			OccurredAt:  result.StartedAt,
			TraceID:     r.newID(),
			JobID:       r.newID(),
			ActorID:     "cycle-scheduler",
			ZoneID:      zone,
			RequestedAt: result.StartedAt,
			Demo:        synthetic,
		}
		// The scheduled seam persists the batch and never reports a job completion.
		// ActorID is descriptive metadata for logs and the batch's origin, never a
		// routing decision: the seam decides the origin, not this string.
		if err := r.publisher.PublishScheduled(ctx, request); err != nil {
			result.Complete = false
			result.Zones = append(result.Zones, ZoneResult{ZoneID: zone, Published: false, Error: err.Error()})
			continue
		}
		result.Zones = append(result.Zones, ZoneResult{ZoneID: zone, Published: true})
	}
	result.FinishedAt = r.now().UTC()
	r.record(result)
	return result
}

// record stores the outcome of one run for Statuses.
func (r *Runner) record(result RunResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	finished := result.FinishedAt
	next := finished.Add(r.interval)
	r.lastRunAt = &finished
	r.nextRunAt = &next
	r.complete = result.Complete
	r.lastError = ""
	if !result.Complete {
		reasons := make([]string, 0, len(result.Zones))
		for _, zone := range result.Zones {
			if zone.Error != "" {
				reasons = append(reasons, zone.ZoneID+": "+zone.Error)
			}
		}
		r.lastError = strings.Join(reasons, "; ")
	}
}

// Statuses returns the freshness of the configured source. It is a one-element
// slice because one runner observes one source; the API keeps the plural shape so
// a multi-source deployment is additive.
func (r *Runner) Statuses() []SourceStatus {
	now := r.now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	status := SourceStatus{SourceID: r.id(), Complete: r.complete, LastError: r.lastError}
	if r.lastRunAt != nil {
		last := *r.lastRunAt
		status.LastRunAt = &last
		age := now.Sub(last).Seconds()
		if age < 0 {
			age = 0
		}
		status.AgeSeconds = &age
	}
	if r.nextRunAt != nil {
		next := *r.nextRunAt
		status.NextRunAt = &next
	}
	return []SourceStatus{status}
}

// Start runs the cycle immediately and then on the configured interval until the
// context is cancelled. A failed run is logged and does not stop the schedule:
// the next tick retries, which is why the refusal never dead-letters.
func (r *Runner) Start(ctx context.Context) {
	run := func() {
		result := r.RunOnce(ctx)
		if !result.Complete {
			slog.Warn("cycle_incomplete", "source_id", result.SourceID, "zones", result.Zones, "error", r.Statuses()[0].LastError)
			return
		}
		slog.Info("cycle_complete", "source_id", result.SourceID, "zones", len(result.Zones), "finished_at", result.FinishedAt)
	}
	run()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
