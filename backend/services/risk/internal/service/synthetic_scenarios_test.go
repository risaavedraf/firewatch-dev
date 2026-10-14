package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/source"
)

// The real provider and classifier are exercised through the publication API.
// The writer is a test boundary, not evidence of a PostgreSQL round trip.
func TestSyntheticScenariosPublishExpectedDemonstrationLevels(t *testing.T) {
	instant := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		id, level string
		refused   bool
	}{
		{"mild", "bajo", false}, {"mixed", "medio", false}, {"dry-windy", "alto", false},
		{"missing-humidity", "desconocido", false}, {"stale", "desconocido", false},
		{"partial-retrieval", "", true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			provider, err := source.NewSyntheticProvider(tc.id, "05101", instant)
			if err != nil {
				t.Fatal(err)
			}
			var baseline string
			var baselineRationale string
			for _, zone := range []string{"05101", "05109"} {
				writer := &stubWriter{}
				request := sampleRequest()
				request.ZoneID = zone
				publisher := Publisher{Provider: provider, Classifier: classify.NewAcademicRuleV1(), Store: writer}
				err := publisher.Publish(context.Background(), request)
				if tc.refused {
					if err == nil || !strings.Contains(err.Error(), "incomplete retrieval") || writer.calls != 0 {
						t.Fatalf("partial retrieval produced stored assessment: calls=%d, err=%v", writer.calls, err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				assessment := writer.assessment
				if writer.calls != 1 || writer.request != request {
					t.Fatal("request not preserved at publication boundary")
				}
				if assessment.RiskLevel != tc.level || !assessment.Demo || assessment.ModelVersion != classify.AcademicRuleV1ID || len(assessment.Factors) != 5 || assessment.Rationale == "" {
					t.Fatalf("unexpected assessment: %+v", assessment)
				}
				if assessment.Policy == nil || assessment.Policy.PolicyID != classify.AcademicRuleV1ID || assessment.Policy.ParamsHash != classify.AcademicRuleV1ParamsHash {
					t.Fatalf("policy not reproducible: %+v", assessment.Policy)
				}
				missing := false
				for _, factor := range assessment.Factors {
					if factor.Name == "humedad_relativa" && !factor.Known {
						missing = true
					}
				}
				if missing != (tc.id == "missing-humidity") {
					t.Fatal("missing-data evidence incorrect")
				}
				if baseline != "" && (baseline != assessment.RiskLevel || baselineRationale != assessment.Rationale) {
					t.Fatal("irrelevant transport zone changed synthetic classification")
				}
				baseline, baselineRationale = assessment.RiskLevel, assessment.Rationale
			}
		})
	}
}

// scenarioInstant is one server-assigned synthetic evaluation instant. It is
// fixed so a scenario publication is exactly replayable and the assertions below
// never depend on the wall clock.
var scenarioInstant = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// scenarioRequest builds the job event the jobs service emits for a synthetic
// profile: the scenario id together with the one instant it persisted for that
// job. The client never supplies the instant, so the test does not either.
func scenarioRequest(scenarioID string, instant time.Time) v1.Event {
	request := sampleRequest()
	request.ScenarioID = scenarioID
	request.EvaluatedAt = &instant
	return request
}

// countingProvider records its fetch calls, so a test can prove that a scenario
// request never consults the configured provider and that invalid metadata is
// refused before any acquisition happens.
type countingProvider struct {
	descriptor observations.SourceDescriptor
	set        observations.ObservationSet
	fetchCalls int
}

func (c *countingProvider) Describe() observations.SourceDescriptor { return c.descriptor }

func (c *countingProvider) Fetch(context.Context, observations.Window) (observations.ObservationSet, error) {
	c.fetchCalls++
	return c.set, nil
}

// samePolicy compares two optional policy references by value, treating two
// nils as equal and never dereferencing a nil.
func samePolicy(a, b *classify.PolicyRef) bool { return reflect.DeepEqual(a, b) }

// TestPublisherSelectsTheScenarioAndMatchesItsOwnProvider protects scenario
// selection end to end: the request's scenario id must select the built-in
// provider for that profile pinned to the job's instant, and the resulting
// assessment must be exactly the one that provider produces directly. The
// configured provider must not be consulted at all, so a scenario request can
// never silently fall back to the legacy fixture, and the scenario identity is
// job metadata that never leaks into a factor.
func TestPublisherSelectsTheScenarioAndMatchesItsOwnProvider(t *testing.T) {
	for _, tc := range []struct{ id, level string }{
		{"mild", string(classify.LevelBajo)},
		{"mixed", string(classify.LevelMedio)},
		{"dry-windy", string(classify.LevelAlto)},
		{"missing-humidity", string(classify.LevelDesconocido)},
		{"stale", string(classify.LevelDesconocido)},
	} {
		t.Run(tc.id, func(t *testing.T) {
			configured := &countingProvider{descriptor: stubDescriptor(true)}
			writer := &stubWriter{}
			request := scenarioRequest(tc.id, scenarioInstant)
			if err := (Publisher{
				Provider:   configured,
				Classifier: classify.NewAcademicRuleV1(),
				Store:      writer,
			}).Publish(context.Background(), request); err != nil {
				t.Fatalf("publish scenario %q: %v", tc.id, err)
			}
			if configured.fetchCalls != 0 {
				t.Fatalf("a scenario request consulted the configured provider %d times", configured.fetchCalls)
			}
			if writer.calls != 1 {
				t.Fatalf("writer calls=%d, want exactly 1 persisted assessment", writer.calls)
			}
			if writer.request != request {
				t.Fatalf("request metadata was not preserved: %+v", writer.request)
			}

			// Baseline: the same profile through its own provider, without the
			// selection path. Equality here proves the selection is a pure routing
			// decision and not a second generator.
			provider, err := source.NewSyntheticProvider(tc.id, request.ZoneID, scenarioInstant)
			if err != nil {
				t.Fatal(err)
			}
			baseline := &stubWriter{}
			if err := (Publisher{
				Provider:   provider,
				Classifier: classify.NewAcademicRuleV1(),
				Store:      baseline,
			}).Publish(context.Background(), sampleRequest()); err != nil {
				t.Fatalf("publish the profile directly: %v", err)
			}

			got, want := writer.assessment, baseline.assessment
			if got.RiskLevel != tc.level || want.RiskLevel != tc.level {
				t.Fatalf("level is %q, want the profile level %q", got.RiskLevel, tc.level)
			}
			if got.RiskLevel != want.RiskLevel || got.Rationale != want.Rationale ||
				!reflect.DeepEqual(got.Factors, want.Factors) || got.Demo != want.Demo ||
				got.ModelVersion != want.ModelVersion || !samePolicy(got.Policy, want.Policy) {
				t.Fatalf("scenario assessment %+v differs from its own-provider baseline %+v", got, want)
			}
			if !got.Demo {
				t.Fatal("a synthetic scenario was persisted as real data")
			}
			encoded, err := json.Marshal(got.Factors)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), tc.id) {
				t.Fatalf("scenario id leaked into factors: %s", encoded)
			}
		})
	}
}

// TestPublisherRefusesThePartialRetrievalScenarioTransiently protects the
// incomplete-acquisition profile through the SELECTION path: an unfinished
// retrieval must not be stored, and because the provider is re-fetched on every
// attempt the refusal stays transient (a later attempt can complete). The
// configured provider must still not be consulted.
func TestPublisherRefusesThePartialRetrievalScenarioTransiently(t *testing.T) {
	configured := &countingProvider{descriptor: stubDescriptor(true)}
	writer := &stubWriter{}
	err := (Publisher{
		Provider:   configured,
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}).Publish(context.Background(), scenarioRequest("partial-retrieval", scenarioInstant))
	if err == nil {
		t.Fatal("the partial-retrieval scenario produced a determined assessment")
	}
	if writer.calls != 0 {
		t.Fatalf("writer calls=%d, an incomplete retrieval must persist nothing", writer.calls)
	}
	if configured.fetchCalls != 0 {
		t.Fatalf("configured provider was consulted %d times", configured.fetchCalls)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "incomplete") {
		t.Fatalf("refusal reason %q does not name the incompleteness", err)
	}
	var permanent interface{ Permanent() bool }
	if errors.As(err, &permanent) && permanent.Permanent() {
		t.Fatalf("an incomplete retrieval was classified as permanent: %v", err)
	}
}

// TestPublisherRefusesInvalidScenarioMetadataBeforeAcquisition protects the
// input contract: the scenario catalogue and the job's server-assigned instant
// are both fixed, so an unknown profile, a missing or zero instant, or an instant
// without a profile can never become valid on a retry. Each is refused BEFORE any
// provider is fetched and before anything is stored, and each error declares
// itself permanent so the consumer dead-letters it after one attempt.
func TestPublisherRefusesInvalidScenarioMetadataBeforeAcquisition(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request v1.Event
	}{
		{"unknown scenario id", scenarioRequest("not-a-scenario", scenarioInstant)},
		{"scenario without an instant", func() v1.Event {
			request := scenarioRequest("mild", scenarioInstant)
			request.EvaluatedAt = nil
			return request
		}()},
		{"scenario with a zero instant", scenarioRequest("mild", time.Time{})},
		{"instant without a scenario", func() v1.Event {
			request := sampleRequest()
			request.EvaluatedAt = &scenarioInstant
			return request
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configured := &countingProvider{descriptor: stubDescriptor(true)}
			writer := &stubWriter{}
			err := (Publisher{
				Provider:   configured,
				Classifier: classify.NewAcademicRuleV1(),
				Store:      writer,
			}).Publish(context.Background(), tc.request)
			if err == nil {
				t.Fatal("invalid scenario metadata was published")
			}
			if configured.fetchCalls != 0 {
				t.Fatalf("provider was fetched %d times before the metadata was validated", configured.fetchCalls)
			}
			if writer.calls != 0 {
				t.Fatalf("writer was reached %d times for invalid metadata", writer.calls)
			}
			var permanent interface{ Permanent() bool }
			if !errors.As(err, &permanent) || !permanent.Permanent() {
				t.Fatalf("err=%v must declare itself permanent", err)
			}
		})
	}
}

// TestPublisherWithoutScenarioKeepsTheConfiguredProviderExactly protects the
// legacy path: a request without scenario metadata must use the configured
// provider unchanged and must not stamp any synthetic metadata onto the stored
// request or result event.
func TestPublisherWithoutScenarioKeepsTheConfiguredProviderExactly(t *testing.T) {
	descriptor := stubDescriptor(true)
	set := observations.ObservationSet{Records: source.FixtureRecords(), Provenance: source.ProvenanceFrom(descriptor)}
	set.Provenance.RetrievedAt = source.SampleInstant
	set.Provenance.Complete = true
	configured := &countingProvider{descriptor: descriptor, set: set}
	writer := &stubWriter{}
	request := sampleRequest()
	if err := (Publisher{
		Provider:   configured,
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}).Publish(context.Background(), request); err != nil {
		t.Fatalf("publish the legacy provider path: %v", err)
	}
	if configured.fetchCalls != 1 {
		t.Fatalf("configured provider was fetched %d times, want exactly 1", configured.fetchCalls)
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls=%d want 1", writer.calls)
	}
	if writer.request.ScenarioID != "" || writer.request.EvaluatedAt != nil {
		t.Fatalf("a legacy request gained synthetic metadata: %+v", writer.request)
	}
	if writer.assessment.RiskLevel != string(classify.LevelAlto) {
		t.Fatalf("legacy fixture level is %q, want %q", writer.assessment.RiskLevel, classify.LevelAlto)
	}
}
