package metrics

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHTTPMetricsRendersClassesBucketsAndErrors protects the wire format: the
// response classes, the single 5xx error series the alert threshold is written
// against, and a cumulative bucket histogram that histogram_quantile can read.
func TestHTTPMetricsRendersClassesBucketsAndErrors(t *testing.T) {
	httpMetrics := &HTTPMetrics{}
	httpMetrics.Observe(200, 12*time.Millisecond)
	httpMetrics.Observe(200, 300*time.Millisecond)
	httpMetrics.Observe(500, 8*time.Second)
	httpMetrics.Observe(404, 5*time.Millisecond)
	text := httpMetrics.Text()
	for _, want := range []string{
		"firewatch_http_responses_total{class=\"2xx\"} 2",
		"firewatch_http_responses_total{class=\"3xx\"} 0",
		"firewatch_http_responses_total{class=\"4xx\"} 1",
		"firewatch_http_responses_total{class=\"5xx\"} 1",
		"firewatch_http_errors_total 1",
		"firewatch_http_request_duration_seconds_bucket{le=\"0.005\"} 1",
		"firewatch_http_request_duration_seconds_bucket{le=\"0.025\"} 2",
		"firewatch_http_request_duration_seconds_bucket{le=\"0.5\"} 3",
		"firewatch_http_request_duration_seconds_bucket{le=\"+Inf\"} 4",
		"firewatch_http_request_duration_seconds_count 4",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("http metrics text %q does not contain %q", text, want)
		}
	}
	if !strings.Contains(text, "firewatch_http_request_duration_seconds_sum ") {
		t.Fatalf("http metrics text %q has no sum line", text)
	}
}

// TestHTTPMetricsZeroValueRendersEmptySeries protects the surface before any
// request: /metrics must render every class and the histogram at zero, not an
// empty body, so a dashboard series exists from the first scrape.
func TestHTTPMetricsZeroValueRendersEmptySeries(t *testing.T) {
	text := (&HTTPMetrics{}).Text()
	for _, want := range []string{
		"firewatch_http_responses_total{class=\"2xx\"} 0",
		"firewatch_http_errors_total 0",
		"firewatch_http_request_duration_seconds_bucket{le=\"+Inf\"} 0",
		"firewatch_http_request_duration_seconds_count 0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("empty http metrics text %q does not contain %q", text, want)
		}
	}
}

// TestStatusClass protects the mapping, including "unknown" for a code outside
// the standard ranges, so no response can silently vanish from the classes.
func TestStatusClass(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{
		{200, "2xx"}, {204, "2xx"}, {301, "3xx"}, {404, "4xx"}, {503, "5xx"}, {0, "unknown"}, {99, "unknown"},
	} {
		if got := statusClass(tc.status); got != tc.want {
			t.Fatalf("statusClass(%d)=%q, want %q", tc.status, got, tc.want)
		}
	}
}

// TestPoolStatsText protects the database-pool gauges, the "data access" signal.
func TestPoolStatsText(t *testing.T) {
	text := PoolStats{Total: 4, Acquired: 1, Idle: 3, Max: 10}.Text()
	for _, want := range []string{
		"firewatch_db_pool_total_conns 4",
		"firewatch_db_pool_acquired_conns 1",
		"firewatch_db_pool_idle_conns 3",
		"firewatch_db_pool_max_conns 10",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pool text %q does not contain %q", text, want)
		}
	}
}

// TestRuntimeTextRendersProcessSignals protects the resource layer and, by
// parsing the values, proves the runtime samples are actually read into real
// numbers rather than degrading to a silent zero.
func TestRuntimeTextRendersProcessSignals(t *testing.T) {
	text := RuntimeText()
	for _, want := range []string{
		"firewatch_process_goroutines ",
		"firewatch_process_cpu_seconds_total ",
		"firewatch_process_memory_bytes ",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("runtime text %q does not contain %q", text, want)
		}
	}
	var goroutines, memory uint64
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("runtime line %q is not a name/value pair", line)
		}
		switch fields[0] {
		case "firewatch_process_goroutines":
			goroutines, _ = strconv.ParseUint(fields[1], 10, 64)
		case "firewatch_process_memory_bytes":
			memory, _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}
	if goroutines < 1 {
		t.Fatalf("goroutines=%d, want at least the running test goroutine", goroutines)
	}
	if memory == 0 {
		t.Fatal("process memory was reported as zero: the runtime sample was not read")
	}
}
