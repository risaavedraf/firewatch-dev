package messaging

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	amqp "github.com/rabbitmq/amqp091-go"
)

// recoveryAck records what the consumer did with one delivery, so a test can
// assert settlement rather than only the handler that ran.
type recoveryAck struct {
	acked, rejected int
	requeued        bool
}

func (a *recoveryAck) Ack(uint64, bool) error { a.acked++; return nil }
func (a *recoveryAck) Nack(uint64, bool, bool) error {
	return nil
}
func (a *recoveryAck) Reject(_ uint64, requeue bool) error {
	a.rejected++
	a.requeued = requeue
	return nil
}

// The dispatch boundary is what keeps a failure report out of the completion
// path and a malformed or unrecognised message out of every writer. Each case
// below asserts the settlement as well as the handler, because acknowledging a
// message nobody processed is the failure mode this boundary exists to prevent.
func TestRecoveryDispatchBoundary(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name, kind, reason string
		attempts           int
		reject             bool
	}{
		{name: "failure", kind: v1.JobFailedType, reason: "retries_exhausted", attempts: 5},
		{name: "unknown type", kind: "future.type.v1", reason: "retries_exhausted", attempts: 5, reject: true},
		{name: "empty reason", kind: v1.JobFailedType, reason: "", attempts: 5, reject: true},
		{name: "zero attempts", kind: v1.JobFailedType, reason: "permanent_error", attempts: 0, reject: true},
		{name: "negative attempts", kind: v1.JobFailedType, reason: "permanent_error", attempts: -1, reject: true},
		{name: "long reason", kind: v1.JobFailedType, reason: strings.Repeat("x", 256), attempts: 1, reject: true},
		{name: "completion", kind: v1.BatchPublishedType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instant := time.Unix(1, 0).UTC()
			event := v1.Event{
				ID: id, Type: tc.kind, JobID: id, TraceID: id,
				ActorID: "stored", ZoneID: "05101", BatchID: id,
				OccurredAt: instant, RequestedAt: instant,
				FailureReason: tc.reason, Attempts: tc.attempts,
			}
			body, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			completed, failed := 0, 0
			b := &Broker{
				Handle: func(context.Context, v1.Event) error {
					completed++
					return nil
				},
				Fail: func(_ context.Context, job, reason string, n int) error {
					failed++
					if job != id || reason != tc.reason || n != tc.attempts {
						t.Fatal("the failure metadata reached the writer changed")
					}
					return nil
				},
			}
			ack := &recoveryAck{}
			deliveries := make(chan amqp.Delivery, 1)
			deliveries <- amqp.Delivery{Body: body, Acknowledger: ack}
			close(deliveries)
			_ = b.consume(context.Background(), deliveries)

			if tc.reject {
				if ack.rejected != 1 || ack.acked != 0 || ack.requeued || completed+failed != 0 {
					t.Fatal("an invalid delivery reached a writer or was acknowledged")
				}
				return
			}
			if ack.acked != 1 || ack.rejected != 0 || completed+failed != 1 {
				t.Fatal("a valid delivery was not processed exactly once")
			}
			if tc.kind == v1.JobFailedType && completed != 0 {
				t.Fatal("a failure report entered the completion path")
			}
			if tc.kind == v1.BatchPublishedType && failed != 0 {
				t.Fatal("a completion entered the failure writer")
			}
		})
	}
}

// The queued-age series is appended after the pre-existing ones, and the unknown
// sentinel must never be mistakable for a young, healthy queue. The legacy prefix
// is written out literally so a reordering or a dropped series fails here.
func TestRecoveryMetricsPreserveLegacyPrefix(t *testing.T) {
	const legacy = "firewatch_http_requests_total 11\n" +
		"firewatch_broker_ready 1\n" +
		"firewatch_events_published_total 21\n" +
		"firewatch_events_processed_total 31\n" +
		"firewatch_events_rejected_total 41\n" +
		"firewatch_dead_letter_depth 0\n" +
		"firewatch_dead_letter_depth_age_seconds 7\n"
	want := legacy + "firewatch_queued_job_age_seconds -1\n"
	if got := MetricsText(11, true, 21, 31, 41, 0, QueuedAgeUnknown, 7); got != want {
		t.Fatalf("metrics=%q; want %q", got, want)
	}
}
