package service

import (
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"github.com/google/uuid"
)

func TestValidateRequestKeepsScenarioMetadataOptional(t *testing.T) {
	base := v1.Event{ID: uuid.NewString(), JobID: uuid.NewString(), TraceID: uuid.NewString(), ActorID: "admin", Type: v1.JobRequestedType, ZoneID: "05101", Demo: true, OccurredAt: time.Now(), RequestedAt: time.Now()}
	// The legacy fixture path stays valid: no scenario metadata is required.
	if err := ValidateRequest(base); err != nil {
		t.Fatalf("legacy request without scenario metadata: %v", err)
	}
	instant := time.Now().UTC()
	scenario := base
	scenario.ScenarioID = "mild"
	scenario.EvaluatedAt = &instant
	if err := ValidateRequest(scenario); err != nil {
		t.Fatalf("a well-formed scenario-bearing request: %v", err)
	}
	// The pairing rule (unknown profile, absent instant, instant without a
	// profile) is a DOMAIN decision the publication path makes before any
	// acquisition, not a transport-shape rule. A scenario without its instant is
	// therefore accepted here and refused permanently there, so this gate cannot
	// start rejecting a message the domain path owns.
	missing := base
	missing.ScenarioID = "mild"
	if err := ValidateRequest(missing); err != nil {
		t.Fatalf("a scenario without an instant must pass the shape gate: %v", err)
	}
}

func TestValidateRequest(t *testing.T) {
	base := v1.Event{ID: uuid.NewString(), JobID: uuid.NewString(), TraceID: uuid.NewString(), ActorID: "admin", Type: v1.JobRequestedType, ZoneID: "valparaiso-demo", Demo: true, OccurredAt: time.Now(), RequestedAt: time.Now()}
	cases := []struct {
		name   string
		mutate func(*v1.Event)
		valid  bool
	}{
		{"demo fixture", func(e *v1.Event) {}, true},
		{"non-demo origin is still well formed", func(e *v1.Event) { e.Demo = false }, true},
		{"wrong event", func(e *v1.Event) { e.Type = v1.BatchPublishedType }, false},
		// A well-formed but unknown zone is ACCEPTED at this shape gate on
		// purpose: membership belongs to the zones catalogue and the
		// batches_zone_id_fkey foreign key, not to a Go allowlist. This case
		// rules out the mutation that reintroduces SupportedZone here and makes
		// the transport reject a zone the catalogue legitimately holds (or
		// re-freezes the accepted set as a literal compiled into the service).
		{"unknown well-formed zone passes the shape gate", func(e *v1.Event) { e.ZoneID = "other" }, true},
		{"empty zone", func(e *v1.Event) { e.ZoneID = "" }, false},
		{"bad id", func(e *v1.Event) { e.ID = "invalid" }, false},
		{"nil id", func(e *v1.Event) { e.ID = uuid.Nil.String() }, false},
		{"missing actor", func(e *v1.Event) { e.ActorID = "" }, false},
		{"future request", func(e *v1.Event) { e.RequestedAt = time.Now().Add(time.Hour) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mutate(&e)
			err := ValidateRequest(e)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
