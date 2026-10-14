package source

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// testProvider is a controllable provider. It lets a test pin the descriptor and
// the set independently, which is what the registry and precedence tests need.
type testProvider struct {
	descriptor observations.SourceDescriptor
	records    []observations.Observation
	complete   bool
	fetchErr   error
}

func (p testProvider) Describe() observations.SourceDescriptor { return p.descriptor }

func (p testProvider) Fetch(context.Context, observations.Window) (observations.ObservationSet, error) {
	if p.fetchErr != nil {
		return observations.ObservationSet{}, p.fetchErr
	}
	return observations.ObservationSet{
		Records: p.records,
		Provenance: observations.Provenance{
			SourceID: p.descriptor.ID,
			Origin:   p.descriptor.Origin,
			Priority: p.descriptor.Priority,
			Complete: p.complete,
		},
	}, nil
}

func validDescriptor(id string) observations.SourceDescriptor {
	return observations.SourceDescriptor{
		ID:          id,
		DisplayName: "Fuente " + id,
		Kind:        observations.SourceKindFile,
		Origin:      observations.OriginPublicOfficial,
		Endpoint:    "https://example.invalid/" + id,
		License:     "CC BY 4.0",
		Attribution: "Organismo (2026). Conjunto de datos. CC BY 4.0.",
		Variables:   observations.VariableNames(),
		Purpose:     "Prueba",
		Priority:    10,
	}
}

func observation(sourceID, variable string, value float64) observations.Observation {
	return observations.Observation{
		SourceID:   sourceID,
		Variable:   variable,
		ZoneID:     "valparaiso",
		ObservedAt: SampleInstant,
		Value:      value,
		Unit:       "C",
		Quality:    observations.QualityOK,
	}
}

func setWith(descriptor observations.SourceDescriptor, complete bool, records ...observations.Observation) observations.ObservationSet {
	return observations.ObservationSet{
		Records: records,
		Provenance: observations.Provenance{
			SourceID:   descriptor.ID,
			SourceKind: descriptor.Kind,
			Origin:     descriptor.Origin,
			Priority:   descriptor.Priority,
			Complete:   complete,
		},
	}
}

func TestRegistryRejectsDuplicatesAndUnknownIdentifiers(t *testing.T) {
	registry := NewRegistry()
	descriptor := validDescriptor("publica-uno")
	if err := registry.Register(testProvider{descriptor: descriptor, complete: true}); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := registry.Register(testProvider{descriptor: descriptor, complete: true}); err == nil {
		t.Fatal("duplicate registration was accepted")
	}
	if _, err := registry.Get("no-existe"); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("Get on an unknown id returned %v, want ErrUnknownProvider", err)
	}
}

// A descriptor missing its licence, attribution or purpose would emit an
// incomplete traceability table, so registration must refuse it.
func TestRegistryRejectsIncompleteDescriptors(t *testing.T) {
	cases := map[string]func(*observations.SourceDescriptor){
		"sin licencia":     func(d *observations.SourceDescriptor) { d.License = "" },
		"sin atribucion":   func(d *observations.SourceDescriptor) { d.Attribution = "" },
		"sin uso":          func(d *observations.SourceDescriptor) { d.Purpose = "" },
		"sin enlace":       func(d *observations.SourceDescriptor) { d.Endpoint = "" },
		"sin variables":    func(d *observations.SourceDescriptor) { d.Variables = nil },
		"origen invalido":  func(d *observations.SourceDescriptor) { d.Origin = "otro" },
		"tipo invalido":    func(d *observations.SourceDescriptor) { d.Kind = "otro" },
		"sin nombre":       func(d *observations.SourceDescriptor) { d.DisplayName = "" },
		"identificador":    func(d *observations.SourceDescriptor) { d.ID = "" },
		"variables vacias": func(d *observations.SourceDescriptor) { d.Variables = []string{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			descriptor := validDescriptor("publica-uno")
			mutate(&descriptor)
			if err := NewRegistry().Register(testProvider{descriptor: descriptor, complete: true}); err == nil {
				t.Fatalf("descriptor with %s was accepted", name)
			}
		})
	}
}

// The traceability table ordering is part of the deliverable, so public sources
// must be listed ahead of client private ones.
func TestRegistryListsPublicSourcesBeforePrivateOnes(t *testing.T) {
	registry := NewRegistry()
	private := validDescriptor("cliente-privada")
	private.Origin = observations.OriginClientPrivate
	private.Priority = 1
	public := validDescriptor("publica-oficial")
	public.Priority = 999
	for _, provider := range []testProvider{
		{descriptor: private, complete: true},
		{descriptor: public, complete: true},
	} {
		if err := registry.Register(provider); err != nil {
			t.Fatalf("registration failed: %v", err)
		}
	}
	descriptors := registry.Descriptors()
	if len(descriptors) != 2 {
		t.Fatalf("got %d descriptors, want 2", len(descriptors))
	}
	if descriptors[0].ID != "publica-oficial" {
		t.Fatalf("first descriptor is %q, want the public source first", descriptors[0].ID)
	}
}

func TestFetchAllReportsFailureWithoutDroppingOtherSources(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testProvider{descriptor: validDescriptor("rompe"), fetchErr: errors.New("boom")}); err != nil {
		t.Fatalf("registration failed: %v", err)
	}
	if err := registry.Register(testProvider{descriptor: validDescriptor("funciona"), complete: true}); err != nil {
		t.Fatalf("registration failed: %v", err)
	}
	sets, err := registry.FetchAll(t.Context(), observations.Window{})
	if err == nil {
		t.Fatal("expected the failing provider to be reported")
	}
	if len(sets) != 1 || sets[0].Provenance.SourceID != "funciona" {
		t.Fatalf("got %d sets, want only the healthy source", len(sets))
	}
}

// The contest requires documented public official data. If precedence were
// driven by priority alone, a client feed could silently overwrite it.
func TestMergePublicBeatsPrivateRegardlessOfPriority(t *testing.T) {
	public := validDescriptor("publica-oficial")
	public.Priority = 999
	private := validDescriptor("cliente-privada")
	private.Origin = observations.OriginClientPrivate
	private.Priority = 1

	result := Merge([]observations.ObservationSet{
		setWith(private, true, observation("cliente-privada", observations.VariableTemperature, 99)),
		setWith(public, true, observation("publica-oficial", observations.VariableTemperature, 1)),
	})
	if len(result.Observations) != 1 {
		t.Fatalf("got %d observations, want 1", len(result.Observations))
	}
	if got := result.Observations[0].Value; got != 1 {
		t.Fatalf("winning value is %v, want the public source value 1", got)
	}
}

func TestMergeUsesPriorityWithinTheSameOrigin(t *testing.T) {
	weak := validDescriptor("secundaria")
	weak.Priority = 50
	strong := validDescriptor("primaria")
	strong.Priority = 5

	result := Merge([]observations.ObservationSet{
		setWith(weak, true, observation("secundaria", observations.VariableWind, 100)),
		setWith(strong, true, observation("primaria", observations.VariableWind, 2)),
	})
	if len(result.Observations) != 1 {
		t.Fatalf("got %d observations, want 1", len(result.Observations))
	}
	if got := result.Observations[0].Value; got != 2 {
		t.Fatalf("winning value is %v, want the lower-priority-number source value 2", got)
	}
}

// The result must not depend on the order the sets arrive in, which is what
// makes a published level reproducible.
func TestMergeIsDeterministicAcrossInputOrder(t *testing.T) {
	first := validDescriptor("alfa")
	second := validDescriptor("beta")
	third := validDescriptor("gama")

	sets := []observations.ObservationSet{
		setWith(first, true,
			observation("alfa", observations.VariableTemperature, 1),
			observation("alfa", observations.VariableWind, 1)),
		setWith(second, true,
			observation("beta", observations.VariableTemperature, 2)),
		setWith(third, true,
			observation("gama", observations.VariableHumidity, 3)),
	}
	reversed := []observations.ObservationSet{sets[2], sets[1], sets[0]}

	forward := Merge(sets)
	backward := Merge(reversed)
	if !reflect.DeepEqual(forward.Observations, backward.Observations) {
		t.Fatalf("merge is order dependent:\nforward  = %+v\nbackward = %+v", forward.Observations, backward.Observations)
	}
}

// A partially paginated retrieval must never be able to satisfy a key by
// accident, and the reason must stay visible rather than being dropped.
func TestMergeSkipsIncompleteRetrievalAndReportsIt(t *testing.T) {
	partial := validDescriptor("parcial")
	whole := validDescriptor("completa")
	whole.Priority = 90

	result := Merge([]observations.ObservationSet{
		setWith(partial, false, observation("parcial", observations.VariableWind, 100)),
		setWith(whole, true, observation("completa", observations.VariableWind, 7)),
	})
	if len(result.Skipped) != 1 || result.Skipped[0].SourceID != "parcial" {
		t.Fatalf("got skipped %+v, want the incomplete source reported", result.Skipped)
	}
	for _, record := range result.Observations {
		if record.SourceID == "parcial" {
			t.Fatal("records from an incomplete retrieval were merged")
		}
	}
	if len(result.Observations) != 1 || result.Observations[0].Value != 7 {
		t.Fatalf("got %+v, want only the complete source record", result.Observations)
	}
}

func TestMergeReportsIncompleteSourceEvenWhenAlone(t *testing.T) {
	partial := validDescriptor("parcial")
	result := Merge([]observations.ObservationSet{
		setWith(partial, false, observation("parcial", observations.VariableWind, 100)),
	})
	if len(result.Observations) != 0 {
		t.Fatalf("got %d observations, want none", len(result.Observations))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("got skipped %+v, want the incomplete source reported", result.Skipped)
	}
}

// The identity of a reading is (variable, zone, observed_at). Truncating the
// instant to the second collapses two distinct readings of the same variable in
// the same zone into one identity, silently losing one of them, and which one
// survives would depend on the order the sets arrived in. Two distinct instants
// inside one second must stay two identities, in a stable order.
func TestMergeKeepsSubSecondIdentitiesOfTheSameVariableAndZone(t *testing.T) {
	descriptor := validDescriptor("publica-uno")
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	early := observation("publica-uno", observations.VariableTemperature, 10)
	early.ObservedAt = base.Add(300 * time.Millisecond)
	late := observation("publica-uno", observations.VariableTemperature, 20)
	late.ObservedAt = base.Add(900 * time.Millisecond)

	forward := Merge([]observations.ObservationSet{setWith(descriptor, true, early, late)})
	if len(forward.Observations) != 2 {
		t.Fatalf("got %d observations, want the two sub-second readings kept: %+v", len(forward.Observations), forward.Observations)
	}
	if !forward.Observations[0].ObservedAt.Equal(early.ObservedAt) || !forward.Observations[1].ObservedAt.Equal(late.ObservedAt) {
		t.Fatalf("kept readings are %+v, want them ordered by instant", forward.Observations)
	}

	// The same two readings in the opposite order must resolve to the same slice.
	backward := Merge([]observations.ObservationSet{setWith(descriptor, true, late, early)})
	if !reflect.DeepEqual(forward.Observations, backward.Observations) {
		t.Fatalf("merge is order dependent inside one second:\nforward  = %+v\nbackward = %+v", forward.Observations, backward.Observations)
	}

	// Whole-second behaviour is unchanged: an exact repeat of the same instant,
	// variable and zone is still one identity resolved by precedence, not two.
	repeated := observation("publica-uno", observations.VariableTemperature, 30)
	repeated.ObservedAt = base
	copyOfRepeated := repeated
	copyOfRepeated.Value = 31
	whole := Merge([]observations.ObservationSet{setWith(descriptor, true, repeated, copyOfRepeated)})
	if len(whole.Observations) != 1 {
		t.Fatalf("got %d observations for one whole-second identity, want 1", len(whole.Observations))
	}
}

func TestRecordsHashIgnoresOrderAndDoesNotMutateInput(t *testing.T) {
	ordered := []observations.Observation{
		observation("alfa", observations.VariableTemperature, 1),
		observation("alfa", observations.VariableWind, 2),
	}
	shuffled := []observations.Observation{ordered[1], ordered[0]}

	firstHash, err := RecordsHash(ordered)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	secondHash, err := RecordsHash(shuffled)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("hash depends on input order: %s vs %s", firstHash, secondHash)
	}
	if !reflect.DeepEqual(ordered, []observations.Observation{
		observation("alfa", observations.VariableTemperature, 1),
		observation("alfa", observations.VariableWind, 2),
	}) {
		t.Fatal("RecordsHash mutated the caller slice")
	}

	changed := []observations.Observation{
		observation("alfa", observations.VariableTemperature, 1),
		observation("alfa", observations.VariableWind, 3),
	}
	changedHash, err := RecordsHash(changed)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if changedHash == firstHash {
		t.Fatal("a different value produced the same hash")
	}
}

// The fixture is the only provider available before any real data exists, so it
// has to satisfy the same validation as a real one.
func TestFixtureProviderIsRegistrableAndDeterministic(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewFixtureProvider()); err != nil {
		t.Fatalf("fixture registration failed: %v", err)
	}
	provider, err := registry.Get("fixture-sample")
	if err != nil {
		t.Fatalf("fixture lookup failed: %v", err)
	}
	first, err := provider.Fetch(t.Context(), observations.Window{})
	if err != nil {
		t.Fatalf("fixture fetch failed: %v", err)
	}
	second, err := provider.Fetch(t.Context(), observations.Window{})
	if err != nil {
		t.Fatalf("fixture fetch failed: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("fixture fetch is not deterministic")
	}
	if !first.Provenance.Complete {
		t.Fatal("fixture retrieval is not marked complete")
	}
	if first.Provenance.NormalizedSHA256 == "" {
		t.Fatal("fixture retrieval carries no normalized hash")
	}
	if len(first.Records) != len(observations.VariableNames()) {
		t.Fatalf("fixture returned %d records, want one per canonical variable", len(first.Records))
	}
}

func TestFixtureFetchFiltersByWindow(t *testing.T) {
	provider := NewFixtureProvider()
	outside := observations.Window{
		From: SampleInstant.Add(24 * time.Hour),
		To:   SampleInstant.Add(48 * time.Hour),
	}
	set, err := provider.Fetch(t.Context(), outside)
	if err != nil {
		t.Fatalf("fixture fetch failed: %v", err)
	}
	if len(set.Records) != 0 {
		t.Fatalf("got %d records outside the window, want none", len(set.Records))
	}
	if !set.Provenance.Complete {
		t.Fatal("an empty but successful retrieval must still be complete")
	}
}

// The persisted demo flag is derived from the provider's provenance, so the copy
// of Synthetic on the provenance must be as reliable as the copies of Origin and
// Priority, and an unset descriptor must stay explicitly false.
func TestProvenanceFromCarriesSynthetic(t *testing.T) {
	descriptor := validDescriptor("sintetica")
	descriptor.Synthetic = true
	if provenance := ProvenanceFrom(descriptor); !provenance.Synthetic {
		t.Fatal("a synthetic descriptor produced a non-synthetic provenance")
	}

	if provenance := ProvenanceFrom(validDescriptor("real")); provenance.Synthetic {
		t.Fatal("a zero-valued descriptor produced a synthetic provenance")
	}
}
