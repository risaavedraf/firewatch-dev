package source

import (
	"math"
	"strings"
	"testing"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// validateSet builds a complete set of five canonical records for one zone, so a
// test mutates exactly one field and the rejection can be attributed to it. The
// validator itself lives in validate.go.
func validateSet() observations.ObservationSet {
	instant := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	records := FixtureRecords()
	for index := range records {
		records[index].ObservedAt = instant
	}
	return observations.ObservationSet{
		Records: records,
		Provenance: observations.Provenance{
			SourceID:    "fixture-sample",
			RetrievedAt: instant,
			Complete:    true,
		},
	}
}

// T2: a complete, well-formed set is accepted whole. This is the control: the
// validator must not reject the deterministic default.
func TestValidateAcceptsACompleteWellFormedSet(t *testing.T) {
	report := Validate(validateSet())
	if !report.Complete() {
		t.Fatalf("a complete set was rejected: %s", report.Reasons())
	}
	if len(report.Accepted) != 5 || len(report.Rejected) != 0 {
		t.Fatalf("accepted=%d rejected=%d, want 5/0", len(report.Accepted), len(report.Rejected))
	}
	if missing := report.Missing; len(missing) != 0 {
		t.Fatalf("a complete set reported missing variables %v", missing)
	}
}

// T2: each rejection cause is exercised separately and named on the report.
func TestValidateRejectsEachInvalidRecordWithARecordedCause(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*observations.Observation)
		records   func(*observations.ObservationSet)
		cause     RejectCause
		detailHas string
	}{
		{
			name:      "missing required field",
			mutate:    func(r *observations.Observation) { r.Unit = "" },
			cause:     CauseMissing,
			detailHas: "unit",
		},
		{
			name:      "missing source id",
			mutate:    func(r *observations.Observation) { r.SourceID = "" },
			cause:     CauseMissing,
			detailHas: "source_id",
		},
		{
			name:      "non-canonical variable",
			mutate:    func(r *observations.Observation) { r.Variable = "inventada" },
			cause:     CauseFormat,
			detailHas: "variable",
		},
		{
			name:      "non-finite value",
			mutate:    func(r *observations.Observation) { r.Value = math.Inf(1) },
			cause:     CauseFormat,
			detailHas: "value",
		},
		{
			name:      "non-canonical quality",
			mutate:    func(r *observations.Observation) { r.Quality = observations.Quality("perfecto") },
			cause:     CauseFormat,
			detailHas: "quality",
		},
		{
			name:      "zero instant",
			mutate:    func(r *observations.Observation) { r.ObservedAt = time.Time{} },
			cause:     CauseDate,
			detailHas: "observed_at",
		},
		{
			name:      "future instant",
			mutate:    func(r *observations.Observation) { r.ObservedAt = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) },
			cause:     CauseDate,
			detailHas: "future",
		},
		{
			name:      "before the plausible epoch",
			mutate:    func(r *observations.Observation) { r.ObservedAt = time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC) },
			cause:     CauseDate,
			detailHas: "before",
		},
		{
			name:      "malformed coordinate zone",
			mutate:    func(r *observations.Observation) { r.ZoneID = "Comuna Valparaíso" },
			cause:     CauseCoordinates,
			detailHas: "zone_id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := validateSet()
			tc.mutate(&set.Records[0])
			report := Validate(set)
			if report.Complete() {
				t.Fatalf("an invalid record was accepted: %+v", set.Records[0])
			}
			if len(report.Accepted) != 4 {
				t.Fatalf("accepted = %d, want the 4 valid records", len(report.Accepted))
			}
			if len(report.Rejected) != 1 {
				t.Fatalf("rejected = %d, want 1", len(report.Rejected))
			}
			rejection := report.Rejected[0]
			if rejection.Cause != tc.cause {
				t.Fatalf("cause = %q, want %q (%s)", rejection.Cause, tc.cause, rejection.Detail)
			}
			if !strings.Contains(rejection.Detail, tc.detailHas) {
				t.Fatalf("detail %q does not name %q", rejection.Detail, tc.detailHas)
			}
			if !strings.Contains(report.Reasons(), string(tc.cause)) {
				t.Fatalf("reasons %q do not carry the cause %q", report.Reasons(), tc.cause)
			}
		})
	}
}

// T2: duplicate identity (variable, zone, instant) is rejected, and the first
// occurrence is the one kept.
func TestValidateRejectsDuplicatesAndKeepsTheFirst(t *testing.T) {
	set := validateSet()
	duplicate := set.Records[0]
	duplicate.Value = 99
	set.Records = append(set.Records, duplicate)

	report := Validate(set)
	if report.Complete() {
		t.Fatal("a duplicated identity was accepted")
	}
	if len(report.Rejected) != 1 || report.Rejected[0].Cause != CauseDuplicate {
		t.Fatalf("rejected = %+v, want one duplicate", report.Rejected)
	}
	if len(report.Accepted) != 5 {
		t.Fatalf("accepted = %d, want the 5 first occurrences", len(report.Accepted))
	}
	for _, record := range report.Accepted {
		if record.Value == 99 {
			t.Fatalf("the later duplicate replaced the first occurrence: %+v", record)
		}
	}
}

// T2: an absent canonical variable is reported as missing but does NOT reject the
// set. Absence is a valid observation of incompleteness; the classifier is what
// turns it into "desconocido", so validating it away would hide a real state.
func TestValidateReportsMissingVariablesWithoutRejectingTheSet(t *testing.T) {
	set := validateSet()
	kept := set.Records[:0]
	for _, record := range set.Records {
		if record.Variable == observations.VariableHumidity {
			continue
		}
		kept = append(kept, record)
	}
	set.Records = kept

	report := Validate(set)
	if !report.Complete() {
		t.Fatalf("an absent variable rejected the set: %s", report.Reasons())
	}
	if len(report.Missing) != 1 || report.Missing[0] != observations.VariableHumidity {
		t.Fatalf("missing = %v, want [%s]", report.Missing, observations.VariableHumidity)
	}
}

// T2: the deterministic default passes its own validator, so shipping validation
// cannot silently stop the fixture from publishing.
func TestFixtureRecordsValidateCleanly(t *testing.T) {
	provider := NewFixtureProvider()
	set, err := provider.Fetch(t.Context(), observations.Window{})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if report := Validate(set); !report.Complete() {
		t.Fatalf("the built-in fixture was rejected by its own validator: %s", report.Reasons())
	}
}
