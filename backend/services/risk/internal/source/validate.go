package source

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// RejectCause is the recorded reason a single record was refused. It is a small
// closed vocabulary rather than a free-form string so a caller can alert or
// aggregate on it without parsing prose.
type RejectCause string

const (
	// CauseMissing means a required field of the record is absent. A record that
	// does not identify itself cannot be used or attributed.
	CauseMissing RejectCause = "missing"
	// CauseFormat means a field is present but not in the canonical vocabulary, or
	// a numeric value is not finite.
	CauseFormat RejectCause = "format"
	// CauseDate means the instant is zero, implausibly old, or later than the
	// retrieval it claims to belong to.
	CauseDate RejectCause = "date"
	// CauseCoordinates means the spatial key (zone_id) is absent or malformed.
	// The canonical observation contract carries the zone identifier as its
	// coordinate reference, so a record that cannot be placed is rejected here.
	CauseCoordinates RejectCause = "coordinates"
	// CauseDuplicate means another record already claimed the identity
	// (variable, zone, instant). The first occurrence is kept.
	CauseDuplicate RejectCause = "duplicate"
)

// Rejection records one refused record and why, with the index so an operator can
// point at the exact entry a source returned.
type Rejection struct {
	Index  int
	Cause  RejectCause
	Detail string
}

// ValidationReport is the outcome of validating one retrieval.
//
// Accepted and Rejected partition the input. Missing lists the canonical
// variables the accepted records do not cover: absence is reported, never
// rejected, because the classifier is what turns an absent input into
// "desconocido" and hiding that state would be the dangerous choice.
type ValidationReport struct {
	Accepted []observations.Observation
	Rejected []Rejection
	Missing  []string
}

// Complete reports whether every record was valid. A batch with any rejection is
// incomplete and must not be published.
func (r ValidationReport) Complete() bool { return len(r.Rejected) == 0 }

// Reasons renders every rejection in a stable order, so the same invalid input
// produces the same operator-facing reason.
func (r ValidationReport) Reasons() string {
	parts := make([]string, 0, len(r.Rejected))
	for _, rejection := range r.Rejected {
		parts = append(parts, fmt.Sprintf("record %d: %s: %s", rejection.Index, rejection.Cause, rejection.Detail))
	}
	return strings.Join(parts, "; ")
}

// zoneIDPattern is the accepted shape of a zone identifier. The seeded catalogue
// uses CUT codes ("05101") and the legacy demo label ("valparaiso-demo"); both
// pass. Whitespace, accents or an empty value do not, because a zone that cannot
// be resolved to the catalogue cannot be placed on the map.
var zoneIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// minObservedAt is the earliest instant this validator considers a real
// observation rather than a parse artefact. It is deliberately far in the past:
// the exact value matters less than refusing a zero or implausible date before it
// reaches the classifier.
var minObservedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// Validate applies the record-level rules required before observations may be
// used: format, date, coordinates, duplicates and missing fields.
//
// The retrieval instant is the upper bound for an observation's own instant: an
// observation stamped after the moment it was retrieved is inconsistent. The
// check is skipped when the provenance carries no retrieval instant, because a
// caller that did not record when it asked cannot be second-guessed.
func Validate(set observations.ObservationSet) ValidationReport {
	report := ValidationReport{
		Accepted: make([]observations.Observation, 0, len(set.Records)),
		Rejected: make([]Rejection, 0),
	}
	seen := make(map[string]bool, len(set.Records))
	present := make(map[string]bool, len(set.Records))
	retrievedAt := set.Provenance.RetrievedAt

	for index, record := range set.Records {
		if rejection, ok := rejectRecord(record, index, retrievedAt, seen); ok {
			report.Rejected = append(report.Rejected, rejection)
			continue
		}
		key := identityKey(record)
		seen[key] = true
		present[record.Variable] = true
		report.Accepted = append(report.Accepted, record)
	}

	for _, variable := range observations.VariableNames() {
		if !present[variable] {
			report.Missing = append(report.Missing, variable)
		}
	}
	sort.Strings(report.Missing)
	return report
}

// rejectRecord returns the rejection for one record, or ok=false when it is
// valid. The checks run in a fixed order so the reported cause is deterministic
// when a record violates more than one rule.
func rejectRecord(record observations.Observation, index int, retrievedAt time.Time, seen map[string]bool) (Rejection, bool) {
	reject := func(cause RejectCause, detail string) (Rejection, bool) {
		return Rejection{Index: index, Cause: cause, Detail: detail}, true
	}

	switch {
	case record.SourceID == "":
		return reject(CauseMissing, "source_id is empty")
	case record.Variable == "":
		return reject(CauseMissing, "variable is empty")
	case record.Unit == "":
		return reject(CauseMissing, "unit is empty")
	case record.ZoneID == "":
		return reject(CauseCoordinates, "zone_id is empty and the record cannot be placed")
	case record.ObservedAt.IsZero():
		return reject(CauseDate, "observed_at is zero")
	case !canonicalVariableName(record.Variable):
		return reject(CauseFormat, fmt.Sprintf("variable %q is not in the canonical vocabulary", record.Variable))
	case !canonicalQuality(record.Quality):
		return reject(CauseFormat, fmt.Sprintf("quality %q is not in the canonical vocabulary", record.Quality))
	case math.IsNaN(record.Value) || math.IsInf(record.Value, 0):
		return reject(CauseFormat, fmt.Sprintf("value for %s is not finite", record.Variable))
	case !zoneIDPattern.MatchString(record.ZoneID):
		return reject(CauseCoordinates, fmt.Sprintf("zone_id %q is not a valid zone identifier", record.ZoneID))
	case record.ObservedAt.Before(minObservedAt):
		return reject(CauseDate, fmt.Sprintf("observed_at %s is before the plausible epoch", record.ObservedAt.UTC().Format(time.RFC3339)))
	case !retrievedAt.IsZero() && record.ObservedAt.After(retrievedAt):
		return reject(CauseDate, fmt.Sprintf("observed_at %s is in the future relative to the retrieval %s", record.ObservedAt.UTC().Format(time.RFC3339), retrievedAt.UTC().Format(time.RFC3339)))
	case seen[identityKey(record)]:
		return reject(CauseDuplicate, fmt.Sprintf("duplicate identity for variable %s in zone %s at %s", record.Variable, record.ZoneID, record.ObservedAt.UTC().Format(time.RFC3339)))
	}
	return Rejection{}, false
}

// identityKey is the record identity used for duplicate detection: one reading of
// one variable, in one zone, at one instant. It mirrors the source.Merge identity
// so the validator and the precedence rule agree on what "the same record" means.
func identityKey(record observations.Observation) string {
	return record.Variable + "\x00" + record.ZoneID + "\x00" + record.ObservedAt.UTC().Format(time.RFC3339Nano)
}

// canonicalQuality reports whether a quality value belongs to the shared
// canonical vocabulary.
func canonicalQuality(quality observations.Quality) bool {
	switch quality {
	case observations.QualityOK, observations.QualityPartial, observations.QualityEstimated, observations.QualityUnknown:
		return true
	}
	return false
}

// canonicalVariableName reports whether a variable name belongs to the shared
// canonical vocabulary of the observations contract.
func canonicalVariableName(name string) bool {
	for _, candidate := range observations.VariableNames() {
		if name == candidate {
			return true
		}
	}
	return false
}
