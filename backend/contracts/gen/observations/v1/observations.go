// Package v1 contains the versioned canonical data contracts for observations,
// source provenance and source descriptors.
//
// These types are pure wire and data shapes. No domain logic is shared here,
// matching the rule already stated in contracts/gen/events/v1. Every service
// that consumes data sources reads these shapes through the same two interfaces:
// a data provider and a classifier.
//
// The same shapes describe three structurally different origins - a public
// file downloader, a client HTTP API and a private database - because a file, an
// HTTP response and a query result are all "a set of records obtained from
// somewhere at some time, with a fingerprint and a scope".
package v1

import "time"

// SourceKind records how a source is reached. It exists so that one provider
// contract covers all three cases and a new source never requires a new shape.
type SourceKind string

const (
	SourceKindFile     SourceKind = "file"
	SourceKindHTTPAPI  SourceKind = "http_api"
	SourceKindDatabase SourceKind = "database"
)

// Origin records who owns a source.
//
// The distinction is load-bearing, not decorative. The contest requires
// documented public official sources, so a client-supplied private feed must
// never be able to obscure or replace the public provenance. See
// Merge in the source package for the precedence rule that enforces this.
type Origin string

const (
	OriginPublicOfficial Origin = "publico_oficial"
	OriginClientPrivate  Origin = "privado_cliente"
)

// Quality describes how much a single observation should be trusted. A value
// that was estimated or is of unknown quality must remain visibly distinct from
// a measured one.
type Quality string

const (
	QualityOK        Quality = "ok"
	QualityPartial   Quality = "partial"
	QualityEstimated Quality = "estimated"
	QualityUnknown   Quality = "unknown"
)

// Window is the closed interval a fetch was asked to cover.
type Window struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Observation is one value of one variable, in one zone, at one instant, from
// one source. SourceID is carried on every record so the provenance of a single
// value stays recoverable after records from several sources are merged.
type Observation struct {
	SourceID   string    `json:"source_id"`
	Variable   string    `json:"variable"`
	ZoneID     string    `json:"zone_id"`
	ObservedAt time.Time `json:"observed_at"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	Quality    Quality   `json:"quality"`
}

// Provenance records exactly what a fetch was and what it returned.
//
// Two fields here differ in meaning from the file case and are the reason this
// struct is not modelled on files:
//
//   - RetrievedAt is when we asked; CutoffAt is which period the data covers, or
//     the source's own as-of date. For a file these coincided. For an API and a
//     database they do not, and recording only the retrieval instant destroys
//     reproducibility because endpoint content changes over time.
//   - PayloadSHA256 is what the server returned; NormalizedSHA256 is the records
//     we actually used. Both are needed: with only one of them nobody can tell
//     whether the source changed or our parsing changed.
//
// Origin and Priority are carried here as well as on the descriptor so that a
// single set is self-describing and can be merged without a registry lookup.
// Providers should stamp both with ProvenanceFrom so the two copies cannot
// diverge.
type Provenance struct {
	SourceID   string     `json:"source_id"`
	SourceKind SourceKind `json:"source_kind"`
	Origin     Origin     `json:"origin"`
	Priority   int        `json:"priority"`
	// Synthetic marks a source whose content is generated for demonstration
	// rather than measured. It is declared by the provider, travels with the
	// provenance, and is what a persisted record derives its "demo" flag from, so
	// the writer never hardcodes that flag and it automatically turns false when a
	// real source is registered.
	Synthetic        bool      `json:"synthetic"`
	Endpoint         string    `json:"endpoint"`
	QueryFingerprint string    `json:"query_fingerprint"`
	RetrievedAt      time.Time `json:"retrieved_at"`
	CutoffAt         time.Time `json:"cutoff_at"`
	Format           string    `json:"format"`
	Records          int       `json:"records"`
	PayloadSHA256    string    `json:"payload_sha256"`
	NormalizedSHA256 string    `json:"normalized_sha256"`
	HTTPStatus       int       `json:"http_status"`
	Complete         bool      `json:"complete"`
}

// SourceDescriptor is the machine-readable description of a source.
//
// A provider publishes one of these through Describe, and the source
// traceability table required by the contest is generated from the registry of
// descriptors. Generating the table from code rather than writing it by hand is
// deliberate: a hand-written table can silently drift from the system, a
// generated one cannot.
//
// Endpoint holds the link for a file or HTTP source and the object or view
// identifier for a database source, so the "enlace" column of the traceability
// table stays meaningful across all three kinds.
type SourceDescriptor struct {
	ID               string     `json:"id"`
	DisplayName      string     `json:"display_name"`
	Kind             SourceKind `json:"kind"`
	Origin           Origin     `json:"origin"`
	Endpoint         string     `json:"endpoint"`
	License          string     `json:"license"`
	Attribution      string     `json:"attribution"`
	Variables        []string   `json:"variables"`
	GeoCoverage      string     `json:"geo_coverage"`
	TemporalCoverage string     `json:"temporal_coverage"`
	Cadence          string     `json:"cadence"`
	Purpose          string     `json:"purpose"`
	Priority         int        `json:"priority"`
	// Synthetic marks a source whose content is generated for demonstration
	// rather than measured. It is declared by the provider, travels with the
	// provenance, and is what a persisted record derives its "demo" flag from, so
	// the writer never hardcodes that flag and it automatically turns false when a
	// real source is registered.
	Synthetic bool `json:"synthetic"`
}

// ObservationSet is the result of one fetch: the requested window, the records,
// and the provenance of that single retrieval.
type ObservationSet struct {
	Window     Window        `json:"window"`
	Records    []Observation `json:"records"`
	Provenance Provenance    `json:"provenance"`
}

// Canonical variable names.
//
// Providers must emit these exact identifiers and the classifier maps them to
// features through this vocabulary. Sharing one list is what keeps a new source
// from needing a special case in classification: a provider that cannot name its
// variables in this vocabulary is not ready to be registered.
const (
	VariableTemperature     = "temperatura"
	VariableHumidity        = "humedad_relativa"
	VariableWind            = "viento"
	VariableVegetation      = "cobertura_vegetal"
	VariableHistoricalFires = "incendios_historicos"
)

// VariableNames returns the canonical variable names in a stable order. The
// slice is a copy, so a caller cannot mutate the shared vocabulary.
func VariableNames() []string {
	return []string{
		VariableTemperature,
		VariableHumidity,
		VariableWind,
		VariableVegetation,
		VariableHistoricalFires,
	}
}
