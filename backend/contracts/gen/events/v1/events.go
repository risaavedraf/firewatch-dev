// Package v1 contains the versioned JSON wire contracts. These types are
// handwritten until schema generation is introduced; no domain logic is shared.
package v1

import "time"

const (
	Exchange           = "firewatch.events"
	JobRequestedType   = "jobs.requested.v1"
	BatchPublishedType = "risk.batch.published.v1"
	// JobFailedType is the cross-service terminal-failure report. A job can die
	// in either consumer and the row must still reach its terminal state, but
	// the risk consumer does not own the job row, so it reports the failure as
	// this event and the jobs consumer applies it. It travels the same way as
	// every other event: through the shared topic exchange, durably from the
	// reporting service's own outbox, with publisher confirms and mandatory
	// routing.
	JobFailedType = "risk.job.failed.v1"
	RiskQueue     = "risk.jobs.v1"
	JobsQueue     = "jobs.results.v1"
)

type Event struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	OccurredAt  time.Time `json:"occurred_at"`
	TraceID     string    `json:"trace_id"`
	JobID       string    `json:"job_id"`
	ActorID     string    `json:"actor_id"`
	ZoneID      string    `json:"zone_id"`
	BatchID     string    `json:"batch_id,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
	Demo        bool      `json:"demo"`
	// Optional synthetic metadata is absent on legacy messages. Scenario-bearing
	// messages require the job's persisted, server-assigned evaluation instant.
	ScenarioID  string     `json:"scenario_id,omitempty"`
	EvaluatedAt *time.Time `json:"evaluated_at,omitempty"`
	// Optional terminal-failure metadata, present only on a JobFailedType
	// message. FailureReason is one of the two stable codes, "permanent_error"
	// or "retries_exhausted": a bounded code an operator can alert on instead of
	// an arbitrary error string. Attempts is the delivery budget the reporting
	// consumer spent. Both are additive and optional (omitempty), so a legacy
	// message keeps exactly the shape it always had.
	//
	// These codes are duplicated in each consumer's messaging adapter rather
	// than shared: the project deliberately keeps local adapters, not a shared
	// SDK, so contracts/gen carries contracts and data, not logic. A change to
	// the vocabulary must be made and tested in BOTH consumers.
	FailureReason string `json:"failure_reason,omitempty"`
	Attempts      int    `json:"attempts,omitempty"`
}

// SyntheticScenarioIDs is the wire enum for demonstration profiles. A copy is
// returned so callers cannot mutate the shared vocabulary. Data generation and
// semantic validation belong in services, not in this wire package.
func SyntheticScenarioIDs() []string {
	return []string{"mild", "mixed", "dry-windy", "missing-humidity", "stale", "partial-retrieval"}
}
