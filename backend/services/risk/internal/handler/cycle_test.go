package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"firewatch.local/risk/internal/cycle"
	"firewatch.local/risk/internal/source"
)

// fakeCycle is the handler's view of the runner: it records run calls and serves
// fixed statuses, so the routes are exercised without a scheduler or a database.
type fakeCycle struct {
	statuses []cycle.SourceStatus
	result   cycle.RunResult
	runs     int
}

func (c *fakeCycle) RunOnce(context.Context) cycle.RunResult { c.runs++; return c.result }

func (c *fakeCycle) Statuses() []cycle.SourceStatus { return c.statuses }

// T4: GET /v1/risk/sources carries the per-source freshness next to the existing
// geography and descriptor, and the previous fields are unchanged.
func TestSourcesExposesCycleFreshness(t *testing.T) {
	last := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	next := last.Add(15 * time.Minute)
	age := 42.5
	runner := &fakeCycle{statuses: []cycle.SourceStatus{{
		SourceID:   "fixture-sample",
		LastRunAt:  &last,
		NextRunAt:  &next,
		Complete:   true,
		AgeSeconds: &age,
	}}}
	handler, token := authorizedServerWith(t, "viewer", &fakeStore{}, runner, source.NewFixtureProvider())
	w := getZones(t, handler, token, "/v1/risk/sources")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var body sourcesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sources) != 1 {
		t.Fatalf("sources = %+v", body.Sources)
	}
	status := body.Sources[0]
	if status.SourceID != "fixture-sample" || !status.Complete || status.LastRunAt == nil || !status.LastRunAt.Equal(last) || status.NextRunAt == nil || status.AgeSeconds == nil || *status.AgeSeconds != age {
		t.Fatalf("freshness = %+v", status)
	}
	// The descriptor and geography halves are unchanged by the addition.
	if body.RiskInput.ID == "" || body.Geography.CutoffSemantics == "" {
		t.Fatalf("existing fields regressed: %+v", body)
	}
}

// T4: with no runner configured the field is an empty list, never null, so the
// UI can render "no sources" without a special case.
func TestSourcesFreshnessIsEmptyWithoutARunner(t *testing.T) {
	handler, token := authorizedServerWith(t, "viewer", &fakeStore{}, nil, source.NewFixtureProvider())
	w := getZones(t, handler, token, "/v1/risk/sources")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["sources"]) != "[]" {
		t.Fatalf("sources = %s, want []", raw["sources"])
	}
}

// T3: the manual trigger is admin-only and runs exactly one cycle.
func TestCycleRunRequiresAdminAndRunsOnce(t *testing.T) {
	last := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, role string
		want       int
		wantRuns   int
	}{
		{"admin runs", "admin", http.StatusOK, 1},
		{"viewer denied", "viewer", http.StatusForbidden, 0},
		{"other denied", "operator", http.StatusForbidden, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeCycle{result: cycle.RunResult{
				SourceID:   "fixture-sample",
				StartedAt:  last,
				FinishedAt: last,
				Complete:   true,
				Zones:      []cycle.ZoneResult{{ZoneID: "valparaiso-demo", Published: true}},
			}}
			handler, token := authorizedServerWith(t, tc.role, &fakeStore{}, runner)
			r := httptest.NewRequest(http.MethodPost, "/v1/risk/cycle/run", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if runner.runs != tc.wantRuns {
				t.Fatalf("runner called %d times, want %d", runner.runs, tc.wantRuns)
			}
			if tc.want != http.StatusOK {
				return
			}
			var result cycle.RunResult
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if !result.Complete || len(result.Zones) != 1 || result.Zones[0].ZoneID != "valparaiso-demo" {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

// T3: without a configured runner the manual trigger fails clearly instead of
// panicking.
func TestCycleRunUnavailableWithoutARunner(t *testing.T) {
	handler, token := authorizedServerWith(t, "admin", &fakeStore{}, nil)
	r := httptest.NewRequest(http.MethodPost, "/v1/risk/cycle/run", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503: %s", w.Code, w.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "cycle_unavailable" {
		t.Fatalf("code = %q", body["code"])
	}
}
