package source_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/source"
)

func TestSyntheticScenariosAreCoherentAndReproducible(t *testing.T) {
	instant := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	catalog := source.SyntheticScenarios()
	wantIDs := []string{"mild", "mixed", "dry-windy", "missing-humidity", "stale", "partial-retrieval"}
	var ids []string
	for _, entry := range catalog {
		ids = append(ids, entry.ID)
	}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("catalog IDs = %v, want %v", ids, wantIDs)
	}
	for _, id := range wantIDs {
		t.Run(id, func(t *testing.T) {
			provider, err := source.NewSyntheticProvider(id, "05101", instant)
			if err != nil {
				t.Fatal(err)
			}
			descriptor := provider.Describe()
			if !descriptor.Synthetic || descriptor.ID == "" || descriptor.Attribution == "" || descriptor.License == "" {
				t.Fatalf("missing synthetic traceability: %+v", descriptor)
			}
			registry := source.NewRegistry()
			if err := registry.Register(provider); err != nil {
				t.Fatalf("register: %v", err)
			}
			first, err := provider.Fetch(context.Background(), observations.Window{})
			if err != nil {
				t.Fatal(err)
			}
			second, err := provider.Fetch(context.Background(), observations.Window{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("same inputs produced different observations/provenance")
			}
			digest, err := source.RecordsHash(first.Records)
			if err != nil || digest != first.Provenance.NormalizedSHA256 {
				t.Fatalf("hash mismatch: %v", err)
			}
			if first.Provenance.SourceID != descriptor.ID || !first.Provenance.Synthetic || !first.Provenance.RetrievedAt.Equal(instant) || first.Provenance.Records != len(first.Records) || first.Provenance.QueryFingerprint == "" {
				t.Fatalf("invalid provenance: %+v", first.Provenance)
			}
			if first.Provenance.PayloadSHA256 != "" || first.Provenance.HTTPStatus != 0 {
				t.Fatal("invented network evidence")
			}
			if first.Provenance.Complete != (id != "partial-retrieval") {
				t.Fatal("incorrect retrieval completeness")
			}
			wantCount := 5
			if id == "missing-humidity" {
				wantCount = 4
			}
			if len(first.Records) != wantCount {
				t.Fatalf("records = %d, want %d", len(first.Records), wantCount)
			}
			seen := map[string]bool{}
			units := map[string]string{observations.VariableTemperature: "C", observations.VariableHumidity: "%", observations.VariableWind: "km/h", observations.VariableVegetation: "%", observations.VariableHistoricalFires: "count"}
			observedAt := instant
			if id == "stale" {
				observedAt = instant.Add(-7 * time.Hour)
			}
			for _, record := range first.Records {
				if seen[record.Variable] || units[record.Variable] == "" || units[record.Variable] != record.Unit {
					t.Fatalf("invalid variable/unit: %+v", record)
				}
				seen[record.Variable] = true
				if record.ZoneID != "05101" || record.SourceID != descriptor.ID || !record.ObservedAt.Equal(observedAt) || record.Quality != observations.QualityEstimated || math.IsNaN(record.Value) || math.IsInf(record.Value, 0) {
					t.Fatalf("invalid synthetic observation: %+v", record)
				}
				if (record.Variable == observations.VariableHumidity || record.Variable == observations.VariableVegetation) && (record.Value < 0 || record.Value > 100) {
					t.Fatal("percentage outside demo domain")
				}
				if (record.Variable == observations.VariableWind || record.Variable == observations.VariableHistoricalFires) && record.Value < 0 {
					t.Fatal("negative wind/fire count")
				}
			}
			if !first.Provenance.CutoffAt.Equal(observedAt) {
				t.Fatal("cutoff does not describe observation instant")
			}
			if id == "missing-humidity" && seen[observations.VariableHumidity] {
				t.Fatal("missing humidity silently replaced")
			}
			// Returned data and catalog entries must not mutate the next retrieval.
			first.Records[0].Value = 999
			third, err := provider.Fetch(context.Background(), observations.Window{})
			if err != nil || !reflect.DeepEqual(second, third) {
				t.Fatal("caller mutated provider state")
			}
		})
	}
	catalog[0].ID = "corrupted"
	if source.SyntheticScenarios()[0].ID != "mild" {
		t.Fatal("caller mutated catalog")
	}
}

func TestSyntheticProviderRejectsInvalidConfiguration(t *testing.T) {
	instant := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		id, zone string
		at       time.Time
	}{
		{"not-a-scenario", "05101", instant}, {"", "05101", instant}, {"mild", "", instant}, {"mild", " ", instant}, {"mild", "05101", time.Time{}},
	} {
		t.Run(tc.id+"/"+tc.zone, func(t *testing.T) {
			if _, err := source.NewSyntheticProvider(tc.id, tc.zone, tc.at); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	var zero source.SyntheticProvider
	if _, err := zero.Fetch(context.Background(), observations.Window{}); err == nil {
		t.Fatal("unconfigured provider accepted")
	}
}

func TestSyntheticProviderHonorsCancellationAndWindow(t *testing.T) {
	instant := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	provider, err := source.NewSyntheticProvider("mild", "05101", instant)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.Fetch(ctx, observations.Window{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	for _, tc := range []struct {
		name   string
		window observations.Window
		count  int
	}{
		{"inclusive", observations.Window{From: instant, To: instant}, 5},
		{"after", observations.Window{From: instant.Add(time.Second)}, 0},
		{"before", observations.Window{To: instant.Add(-time.Second)}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, err := provider.Fetch(context.Background(), tc.window)
			if err != nil || len(set.Records) != tc.count || set.Window != tc.window || !set.Provenance.Complete || set.Provenance.Records != tc.count {
				t.Fatalf("window retrieval: %+v, %v", set, err)
			}
			hash, err := source.RecordsHash(set.Records)
			if err != nil || hash != set.Provenance.NormalizedSHA256 {
				t.Fatal("filtered hash mismatch")
			}
		})
	}
}
