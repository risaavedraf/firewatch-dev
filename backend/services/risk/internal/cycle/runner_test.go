package cycle

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/service"
	"firewatch.local/risk/internal/source"
)

// stubPublisher records the requests one cycle published and can be made to fail
// on a chosen zone, so ordering and partial failure are both observable. It
// implements the scheduled seam only: the cycle has no way to ask for a job
// completion.
type stubPublisher struct {
	calls int
	zones []string
	fail  map[string]error
}

func (p *stubPublisher) PublishScheduled(_ context.Context, request v1.Event) error {
	p.calls++
	p.zones = append(p.zones, request.ZoneID)
	if err := p.fail[request.ZoneID]; err != nil {
		return err
	}
	return nil
}

type stubSource struct{ descriptor observations.SourceDescriptor }

func (s stubSource) Describe() observations.SourceDescriptor { return s.descriptor }

func testSource() stubSource {
	descriptor := source.NewFixtureProvider().Describe()
	return stubSource{descriptor: descriptor}
}

// newTestRunner builds a runner with an injected clock and identifier generator,
// so a run is byte-for-byte reproducible and a test never sleeps. The zones
// default to the demo zone and can be overridden per test.
func newTestRunner(publisher Publisher, zones ...string) (*Runner, *time.Time) {
	if len(zones) == 0 {
		zones = []string{"valparaiso-demo"}
	}
	instant := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	clock := instant
	ids := 0
	runner := New(testSource(), publisher, Options{
		Zones:    zones,
		Interval: 15 * time.Minute,
		Now:      func() time.Time { return clock },
		NewID:    func() string { ids++; return "id-" + string(rune('0'+ids)) },
	})
	return runner, &clock
}

// T3: one run executes the whole cycle step for every configured zone and
// records a complete, fresh state.
func TestRunnerRunsEveryZoneAndRecordsFreshness(t *testing.T) {
	publisher := &stubPublisher{}
	runner, clock := newTestRunner(publisher)
	*clock = clock.Add(30 * time.Second)

	result := runner.RunOnce(context.Background())
	if !result.Complete || result.SourceID != "fixture-sample" {
		t.Fatalf("result = %+v", result)
	}
	if publisher.calls != 1 || len(result.Zones) != 1 || !result.Zones[0].Published {
		t.Fatalf("zones = %+v, calls = %d", result.Zones, publisher.calls)
	}

	statuses := runner.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("statuses = %+v", statuses)
	}
	status := statuses[0]
	if status.SourceID != "fixture-sample" || !status.Complete {
		t.Fatalf("status = %+v", status)
	}
	if status.LastRunAt == nil || !status.LastRunAt.Equal(result.FinishedAt) {
		t.Fatalf("last run = %v, want %v", status.LastRunAt, result.FinishedAt)
	}
	if status.NextRunAt == nil || !status.NextRunAt.Equal(result.FinishedAt.Add(15*time.Minute)) {
		t.Fatalf("next run = %v, want +15m", status.NextRunAt)
	}
	if status.AgeSeconds == nil || *status.AgeSeconds < 0 {
		t.Fatalf("age = %v, want a non-negative age", status.AgeSeconds)
	}
	if status.LastError != "" {
		t.Fatalf("a successful run reported error %q", status.LastError)
	}
}

// T3: a zone that fails does not abort the rest, and its reason is recorded so
// the freshness panel can explain an incomplete cycle.
func TestRunnerRecordsAFailedZoneWithoutAbortingTheRest(t *testing.T) {
	publisher := &stubPublisher{fail: map[string]error{"zona-b": errors.New("provider refused")}}
	runner, _ := newTestRunner(publisher, "zona-a", "zona-b", "zona-c")

	result := runner.RunOnce(context.Background())
	if result.Complete {
		t.Fatal("a run with a failed zone reported itself complete")
	}
	if publisher.calls != 3 {
		t.Fatalf("published %d zones, want all 3 attempted", publisher.calls)
	}
	if len(result.Zones) != 3 || !result.Zones[0].Published || result.Zones[1].Published || !result.Zones[2].Published {
		t.Fatalf("zones = %+v", result.Zones)
	}
	if !strings.Contains(result.Zones[1].Error, "provider refused") {
		t.Fatalf("failed zone lost its reason: %+v", result.Zones[1])
	}
	statuses := runner.Statuses()
	if len(statuses) != 1 || statuses[0].Complete || !strings.Contains(statuses[0].LastError, "provider refused") {
		t.Fatalf("status did not record the failure: %+v", statuses)
	}
}

// T4: before any run there is no last run and no invented age; the route must be
// able to say "never run" instead of "fresh at zero".
func TestStatusesBeforeAnyRunReportNeverRun(t *testing.T) {
	runner, _ := newTestRunner(&stubPublisher{})
	statuses := runner.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("statuses = %+v", statuses)
	}
	status := statuses[0]
	if status.LastRunAt != nil || status.AgeSeconds != nil || status.NextRunAt != nil || status.Complete || status.LastError != "" {
		t.Fatalf("never-run status invented state: %+v", status)
	}
}

// T3 end to end without a database: the runner drives the real Publisher over the
// deterministic fixture, so ingest, validation, classification and publication
// all execute in one run and one assessed batch is persisted through the
// automatic-cycle seam.
func TestRunnerDrivesTheRealPublisherOverTheFixture(t *testing.T) {
	writer := &recordingWriter{}
	publisher := &service.Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}
	instant := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	runner := New(source.NewFixtureProvider(), publisher, Options{
		Zones:    []string{"valparaiso-demo"},
		Interval: 15 * time.Minute,
		Now:      func() time.Time { return instant },
	})
	result := runner.RunOnce(context.Background())
	if !result.Complete || writer.scheduled != 1 {
		t.Fatalf("result = %+v, persisted = %d", result, writer.scheduled)
	}
	if writer.assessment.ModelVersion != classify.AcademicRuleV1ID || writer.assessment.RiskLevel != string(classify.LevelAlto) {
		t.Fatalf("persisted assessment = %+v", writer.assessment)
	}
	if writer.request.ZoneID != "valparaiso-demo" {
		t.Fatalf("request zone = %q", writer.request.ZoneID)
	}
}

// S1 regression: the automatic cycle must never report a job completion. Jobs
// never stored the job id the cycle mints, so a risk.batch.published.v1 naming it
// is refused as an unknown job and dead-lettered permanently - once per zone per
// cycle, forever. The batch itself must still be persisted for every zone, with
// the same validation, classification and policy checks an explicit request
// runs, so suppressing the completion cannot become a way to skip the pipeline.
func TestAutomaticCyclePublicationNeverReportsAJobCompletion(t *testing.T) {
	writer := &recordingWriter{}
	publisher := &service.Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}
	instant := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	runner := New(source.NewFixtureProvider(), publisher, Options{
		Zones:    []string{"valparaiso-demo", "zona-b"},
		Interval: 15 * time.Minute,
		Now:      func() time.Time { return instant },
	})
	result := runner.RunOnce(context.Background())
	if !result.Complete {
		t.Fatalf("result = %+v", result)
	}
	if writer.scheduled != 2 {
		t.Fatalf("persisted %d scheduled batches, want one per configured zone (2)", writer.scheduled)
	}
	if writer.completions != 0 {
		t.Fatalf("the automatic cycle reported %d job completions, want 0: no job row exists for a cycle-minted id", writer.completions)
	}
	if writer.assessment.RiskLevel != string(classify.LevelAlto) || writer.assessment.Policy == nil {
		t.Fatalf("the scheduled path skipped the shared assessment checks: %+v", writer.assessment)
	}
}

// recordingWriter is the minimal BatchWriter the runner test needs; it proves the
// real publication path was reached without a database. The two persistence
// seams are counted separately: Publish settles an explicitly requested job and
// PublishScheduled persists an automatic cycle's batch without reporting one, so
// a test can prove the runner chose the scheduled seam.
type recordingWriter struct {
	completions int
	scheduled   int
	request     v1.Event
	assessment  repository.Assessment
}

func (w *recordingWriter) Publish(_ context.Context, request v1.Event, assessment repository.Assessment) error {
	w.completions++
	w.request = request
	w.assessment = assessment
	return nil
}

func (w *recordingWriter) PublishScheduled(_ context.Context, request v1.Event, assessment repository.Assessment) error {
	w.scheduled++
	w.request = request
	w.assessment = assessment
	return nil
}

// A run that produces no zones still reports a deterministic empty result.
func TestRunnerWithNoZonesIsAnEmptyCompleteRun(t *testing.T) {
	publisher := &stubPublisher{}
	instant := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	runner := New(testSource(), publisher, Options{Zones: []string{}, Interval: 15 * time.Minute, Now: func() time.Time { return instant }})
	result := runner.RunOnce(context.Background())
	if !result.Complete || len(result.Zones) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if !reflect.DeepEqual(result.Zones, []ZoneResult{}) {
		t.Fatalf("zones = %#v, want an empty non-nil slice", result.Zones)
	}
}
