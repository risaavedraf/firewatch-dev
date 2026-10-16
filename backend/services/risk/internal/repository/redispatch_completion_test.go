package repository

import (
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"github.com/google/uuid"
)

// TestRedispatchCompletionNamesTheStoredBatch pins the pure helper behind S6: an
// operator re-dispatch carries a fresh event id, so the completion it triggers
// must name the batch that is already stored and copy the request envelope
// unchanged. Minting a new batch id here would report a batch that was never
// written, and dropping the envelope would strip the trace and scenario metadata
// the jobs consumer needs to settle the job.
func TestRedispatchCompletionNamesTheStoredBatch(t *testing.T) {
	instant := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	request := v1.Event{
		ID:          uuid.NewString(),
		Type:        v1.JobRequestedType,
		OccurredAt:  time.Now().UTC(),
		TraceID:     uuid.NewString(),
		JobID:       uuid.NewString(),
		ActorID:     "operator",
		ZoneID:      "valparaiso-demo",
		RequestedAt: time.Now().UTC().Add(-2 * time.Minute),
		Demo:        true,
		ScenarioID:  "dry-windy",
		EvaluatedAt: &instant,
	}
	existingBatchID := uuid.NewString()
	completion := redispatchCompletion(request, existingBatchID)

	if completion.TraceID != request.TraceID || completion.JobID != request.JobID || completion.ActorID != request.ActorID || completion.ZoneID != request.ZoneID || !completion.RequestedAt.Equal(request.RequestedAt) || completion.Demo != request.Demo {
		t.Fatalf("completion did not copy the request envelope: %+v", completion)
	}
	if completion.ScenarioID != request.ScenarioID || completion.EvaluatedAt == nil || !completion.EvaluatedAt.Equal(instant) {
		t.Fatalf("completion dropped the scenario metadata: %+v", completion)
	}
	if completion.Type != v1.BatchPublishedType {
		t.Fatalf("completion type is %q, want %q", completion.Type, v1.BatchPublishedType)
	}
	if completion.BatchID != existingBatchID {
		t.Fatalf("completion names batch %q, want the stored %q", completion.BatchID, existingBatchID)
	}
	if completion.BatchID == request.ID {
		t.Fatal("completion reused the request event id as a batch id")
	}
	if id, err := uuid.Parse(completion.ID); err != nil || id == uuid.Nil {
		t.Fatalf("completion id %q is not a UUID: %v", completion.ID, err)
	}
	if completion.ID == request.ID {
		t.Fatal("completion reused the request's event id")
	}
	if completion.OccurredAt.IsZero() {
		t.Fatal("completion has no occurrence instant")
	}
}
