package service

import (
	"errors"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"github.com/google/uuid"
)

// ValidateRequest is the transport guard for a job-requested event. It is not a
// classifier and performs no I/O: it only decides whether the broker may run the
// callback, and it fails closed. A message it rejects is dead-lettered rather
// than processed.
//
// The zone is checked for SHAPE only: an empty ZoneID is a missing required field
// and is invalid. Membership is deliberately NOT validated here. The set of
// valid zones is the catalogue in the zones table, and batches.zone_id is a
// foreign key into zones(code) (migration 005), so a well-formed zone that passes
// this shape rule but is absent from zones is rejected by the foreign key when
// the batch is written. An application allowlist here would recreate the
// hardcoded literal the schema is meant to own.
//
// The event's origin is not part of its shape. The demo flag used to be required
// here, but it describes where the request came from, not whether the message is
// well formed: the persisted batches.demo column is derived from the provider's
// provenance by the publication path, never from this flag. A well-formed
// request with demo=false is therefore accepted. A genuinely malformed event -
// wrong type, a non-UUID identifier, an empty or oversized actor, a zero instant,
// or a request more than a minute in the future - still fails closed and is still
// sent to the dead-letter queue by the broker.
func ValidateRequest(event v1.Event) error {
	if event.Type != v1.JobRequestedType || event.ZoneID == "" || event.ActorID == "" || len(event.ActorID) > 255 || event.RequestedAt.IsZero() || event.OccurredAt.IsZero() || event.RequestedAt.After(time.Now().Add(time.Minute)) {
		return errors.New("invalid event")
	}
	for _, value := range []string{event.ID, event.JobID, event.TraceID} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil {
			return errors.New("invalid event identifier")
		}
	}
	return nil
}
