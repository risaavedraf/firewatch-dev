// Package v1 contains the versioned JSON wire contracts. These types are
// handwritten until schema generation is introduced; no domain logic is shared.
package v1

import "time"

const (
	Exchange           = "firewatch.events"
	JobRequestedType   = "jobs.requested.v1"
	BatchPublishedType = "risk.batch.published.v1"
	RiskQueue          = "risk.jobs.v1"
	JobsQueue          = "jobs.results.v1"
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
}

// SyntheticScenarioIDs is the wire enum for demonstration profiles. A copy is
// returned so callers cannot mutate the shared vocabulary. Data generation and
// semantic validation belong in services, not in this wire package.
func SyntheticScenarioIDs() []string {
	return []string{"mild", "mixed", "dry-windy", "missing-humidity", "stale", "partial-retrieval"}
}
