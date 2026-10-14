// Package source defines the single data-provider contract used by the risk
// service, a registry of providers, and the deterministic precedence rule that
// resolves overlapping observations.
//
// The contract is deliberately narrow. A provider answers two questions: what
// are you (Describe), and give me your records for this window (Fetch).
// Normalization of a source-specific payload happens inside each provider, so
// everything downstream only ever sees canonical observations. Without that
// boundary the client API's JSON shape would leak into the classifier and every
// new source would force a change to classification.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// Provider is implemented once per data origin: a public-source downloader, a
// client HTTP API, or a private database.
type Provider interface {
	// Describe returns the machine-readable description of this source. It is
	// the single input of the contest's source traceability table, so the table
	// is generated from code and cannot drift from the running system.
	Describe() observations.SourceDescriptor

	// Fetch returns the records for the requested window together with the
	// provenance of that one retrieval. A partially paginated retrieval must
	// report Provenance.Complete as false rather than silently looking whole.
	Fetch(ctx context.Context, window observations.Window) (observations.ObservationSet, error)
}

// ErrUnknownProvider is returned when a registered identifier is requested.
var ErrUnknownProvider = errors.New("unknown data provider")

// ProvenanceFrom stamps the precedence fields of a descriptor onto a provenance
// value. Providers should build their provenance through this helper so the
// copies of Origin, Priority and Synthetic cannot diverge between descriptor and
// set.
func ProvenanceFrom(descriptor observations.SourceDescriptor) observations.Provenance {
	return observations.Provenance{
		SourceID:   descriptor.ID,
		SourceKind: descriptor.Kind,
		Origin:     descriptor.Origin,
		Priority:   descriptor.Priority,
		Endpoint:   descriptor.Endpoint,
		Synthetic:  descriptor.Synthetic,
	}
}

// Registry holds the known providers. It is not safe for concurrent mutation;
// callers register everything during startup, before serving traffic.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: map[string]Provider{}}
}

// Register adds a provider after validating its descriptor.
//
// Validation is strict on purpose. A descriptor missing its licence, its
// attribution or its purpose would produce an incomplete traceability table, and
// the table is a graded deliverable rather than a convenience. Attribution in
// particular is a licence obligation (CC BY), not optional metadata.
func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return errors.New("nil provider")
	}
	descriptor := provider.Describe()
	if err := validateDescriptor(descriptor); err != nil {
		return fmt.Errorf("invalid source descriptor: %w", err)
	}
	if _, exists := r.providers[descriptor.ID]; exists {
		return fmt.Errorf("data provider %q is already registered", descriptor.ID)
	}
	r.providers[descriptor.ID] = provider
	return nil
}

// Get returns the provider registered under id.
func (r *Registry) Get(id string) (Provider, error) {
	provider, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, id)
	}
	return provider, nil
}

// Descriptors returns every descriptor in the precedence order used by Merge,
// so the traceability table is emitted in a stable, explainable sequence.
func (r *Registry) Descriptors() []observations.SourceDescriptor {
	descriptors := make([]observations.SourceDescriptor, 0, len(r.providers))
	for _, provider := range r.providers {
		descriptors = append(descriptors, provider.Describe())
	}
	sort.Slice(descriptors, func(i, j int) bool {
		return precedenceOf(descriptors[i]).less(precedenceOf(descriptors[j]))
	})
	return descriptors
}

// FetchAll calls every registered provider for the same window, in descriptor
// order, and returns the raw sets. A failing provider does not abort the rest:
// each failure is reported alongside the sets that succeeded so a caller can
// decide whether a missing source is tolerable for the assessment at hand.
func (r *Registry) FetchAll(ctx context.Context, window observations.Window) ([]observations.ObservationSet, error) {
	sets := make([]observations.ObservationSet, 0, len(r.providers))
	var failures []error
	for _, descriptor := range r.Descriptors() {
		provider := r.providers[descriptor.ID]
		set, err := provider.Fetch(ctx, window)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", descriptor.ID, err))
			continue
		}
		sets = append(sets, set)
	}
	return sets, errors.Join(failures...)
}

// Skip records a set that Merge refused to use, and why. Discards are reported
// rather than dropped silently: a partial retrieval that quietly passes as
// complete is exactly the kind of defect a traceability table exists to expose.
type Skip struct {
	SourceID string
	Reason   string
}

// MergeResult is the outcome of Merge.
type MergeResult struct {
	Observations []observations.Observation
	Skipped      []Skip
}

// Merge resolves observations from several sets into one deterministic slice.
//
// Precedence, applied to the identity key of (variable, zone, observed_at):
//
//  1. A public official source always wins over a client private source,
//     regardless of priority. The contest requires documented public official
//     data, and that requirement must not be erasable by switching on a client
//     feed: if precedence were driven only by priority, plugging the client in
//     could silently overwrite the public provenance the deliverable depends on.
//  2. Inside the same origin, a lower Priority value wins.
//  3. Remaining ties are broken by SourceID, so the result never depends on map
//     or slice iteration order.
//
// Sets whose provenance reports Complete as false are excluded and recorded in
// Skipped: a partial retrieval must not be able to satisfy a key by accident.
func Merge(sets []observations.ObservationSet) MergeResult {
	result := MergeResult{}
	winner := map[obsKey]observations.Observation{}
	winnerRank := map[obsKey]precedence{}

	for _, set := range sets {
		if !set.Provenance.Complete {
			result.Skipped = append(result.Skipped, Skip{
				SourceID: set.Provenance.SourceID,
				Reason:   "incomplete retrieval",
			})
			continue
		}
		candidate := precedenceOf(set.Provenance)
		for _, observation := range set.Records {
			key := obsKey{
				variable: observation.Variable,
				zoneID:   observation.ZoneID,
				// The instant keeps its full precision, normalized to UTC and
				// stripped of any monotonic reading so two representations of
				// the same instant are the same map key. Truncating it to the
				// second would collapse two distinct readings of one variable
				// in one zone into one identity and silently drop one of them.
				observedAt: normalizeInstant(observation.ObservedAt),
			}
			current, seen := winnerRank[key]
			if !seen || candidate.less(current) {
				winner[key] = observation
				winnerRank[key] = candidate
			}
		}
	}

	keys := make([]obsKey, 0, len(winner))
	for key := range winner {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].less(keys[j]) })
	for _, key := range keys {
		result.Observations = append(result.Observations, winner[key])
	}
	sort.Slice(result.Skipped, func(i, j int) bool {
		return result.Skipped[i].SourceID < result.Skipped[j].SourceID
	})
	return result
}

// RecordsHash returns the stable fingerprint of a record set. It is the
// normalized_sha256 of a Provenance: the proof of which records an assessment
// used. It is computed over a sorted copy, so the hash does not depend on input
// order and the caller's slice is never mutated.
func RecordsHash(records []observations.Observation) (string, error) {
	ordered := make([]observations.Observation, len(records))
	copy(ordered, records)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Variable != ordered[j].Variable {
			return ordered[i].Variable < ordered[j].Variable
		}
		if ordered[i].ZoneID != ordered[j].ZoneID {
			return ordered[i].ZoneID < ordered[j].ZoneID
		}
		if !ordered[i].ObservedAt.Equal(ordered[j].ObservedAt) {
			return ordered[i].ObservedAt.Before(ordered[j].ObservedAt)
		}
		return ordered[i].SourceID < ordered[j].SourceID
	})
	encoded, err := json.Marshal(ordered)
	if err != nil {
		return "", fmt.Errorf("hash records: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// obsKey is the identity of one reading: (variable, zone, instant). The instant
// carries its full precision, so two readings of the same variable in the same
// zone inside the same second are two identities rather than one.
type obsKey struct {
	variable   string
	zoneID     string
	observedAt time.Time
}

// normalizeInstant puts an instant into the single representation map keys use:
// UTC, with any monotonic clock reading stripped, so equality is the instant
// itself and not the way it happened to be constructed.
func normalizeInstant(instant time.Time) time.Time {
	return instant.Round(0).UTC()
}

func (a obsKey) less(b obsKey) bool {
	if a.variable != b.variable {
		return a.variable < b.variable
	}
	if a.zoneID != b.zoneID {
		return a.zoneID < b.zoneID
	}
	return a.observedAt.Before(b.observedAt)
}

// precedence is the total order that makes Merge independent of iteration order.
type precedence struct {
	origin   observations.Origin
	priority int
	id       string
}

func (a precedence) less(b precedence) bool {
	aRank, bRank := originRank(a.origin), originRank(b.origin)
	if aRank != bRank {
		return aRank < bRank
	}
	if a.priority != b.priority {
		return a.priority < b.priority
	}
	return a.id < b.id
}

func precedenceOf(value any) precedence {
	switch typed := value.(type) {
	case observations.SourceDescriptor:
		return precedence{origin: typed.Origin, priority: typed.Priority, id: typed.ID}
	case observations.Provenance:
		return precedence{origin: typed.Origin, priority: typed.Priority, id: typed.SourceID}
	default:
		return precedence{}
	}
}

// originRank encodes rule 1: public official data outranks client private data.
func originRank(origin observations.Origin) int {
	if origin == observations.OriginPublicOfficial {
		return 0
	}
	return 1
}

func validateDescriptor(descriptor observations.SourceDescriptor) error {
	if descriptor.ID == "" {
		return errors.New("id is empty")
	}
	if descriptor.DisplayName == "" {
		return errors.New("display_name is empty")
	}
	switch descriptor.Kind {
	case observations.SourceKindFile, observations.SourceKindHTTPAPI, observations.SourceKindDatabase:
	default:
		return fmt.Errorf("unsupported source_kind %q", descriptor.Kind)
	}
	switch descriptor.Origin {
	case observations.OriginPublicOfficial, observations.OriginClientPrivate:
	default:
		return fmt.Errorf("unsupported origin %q", descriptor.Origin)
	}
	if descriptor.Endpoint == "" {
		return errors.New("endpoint is empty")
	}
	if descriptor.License == "" {
		return errors.New("license is empty")
	}
	if descriptor.Attribution == "" {
		return errors.New("attribution is empty")
	}
	if len(descriptor.Variables) == 0 {
		return errors.New("variables is empty")
	}
	if descriptor.Purpose == "" {
		return errors.New("purpose is empty")
	}
	return nil
}
