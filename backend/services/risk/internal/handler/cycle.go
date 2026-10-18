package handler

import (
	"context"
	"net/http"

	"firewatch.local/risk/internal/cycle"
)

// CycleRunner is the handler's view of the 15 minute cycle: it can report the
// freshness of the active source and run one cycle synchronously for the manual
// demo trigger. Keeping it an interface means the routes are testable without a
// scheduler and the SQL stays out of the transport.
type CycleRunner interface {
	RunOnce(ctx context.Context) cycle.RunResult
	Statuses() []cycle.SourceStatus
}

// cycleRun serves POST /v1/risk/cycle/run. It is wrapped in requireAdmin by the
// router. The run is synchronous so the demo shows its result immediately; with
// no runner configured the route fails clearly instead of panicking.
func cycleRun(runner CycleRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if runner == nil {
			fail(w, http.StatusServiceUnavailable, "cycle_unavailable")
			return
		}
		respond(w, http.StatusOK, runner.RunOnce(r.Context()))
	}
}
