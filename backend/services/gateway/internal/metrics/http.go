// Package metrics holds the gateway's own observability signals: the HTTP
// traffic it answers and the process resources it costs, which /metrics exposes.
// It is the same pattern the risk and jobs services use, duplicated rather than
// imported because each service is its own Go module.
//
// The gateway proxies rather than computes, so it has no model-outcome layer:
// its layers are the service (responses by class, the 5xx error total and a
// request-duration histogram) and the process. Latency measured here is the
// whole hop the page waits on, including the upstream's own time, which is
// exactly the number the p95 alert is written against; the upstream's internal
// cost stays visible in that service's own /metrics.
package metrics

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// durationBuckets are the histogram bucket upper bounds in seconds. They are the
// Prometheus default set: fine enough around tens of milliseconds to make a p95
// useful for a page request, and coarse enough above a second to catch the tail
// the SLO is about (p95 < 500 ms, maximum 2 s under normal conditions).
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// statusClasses are the response classes the surface always reports, even at
// zero, so a dashboard series never appears or disappears with traffic.
var statusClasses = []string{"2xx", "3xx", "4xx", "5xx"}

// HTTPMetrics records the gateway's own HTTP traffic: responses by status class
// and a request-duration histogram. It is what makes a p95 and an error rate
// observable on this hop without changing a single handler.
//
// It is safe for concurrent use: the middleware records from many request
// goroutines while /metrics reads from one.
type HTTPMetrics struct {
	mu        sync.Mutex
	responses map[string]uint64
	durations histogram
}

// Observe records one completed response: its status code and how long it took.
func (m *HTTPMetrics) Observe(status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.responses == nil {
		m.responses = map[string]uint64{}
		m.durations = newHistogram(durationBuckets)
	}
	m.responses[statusClass(status)]++
	m.durations.observe(duration.Seconds())
}

// Text renders the HTTP traffic lines. The 5xx class is additionally exposed as
// firewatch_http_errors_total, because that is the single series the agreed error
// alert threshold is written against.
func (m *HTTPMetrics) Text() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.durations.buckets == nil {
		m.durations = newHistogram(durationBuckets)
	}
	var out strings.Builder
	var errors uint64
	for _, class := range statusClasses {
		count := m.responses[class]
		if class == "5xx" {
			errors = count
		}
		fmt.Fprintf(&out, "firewatch_http_responses_total{class=\"%s\"} %d\n", class, count)
	}
	fmt.Fprintf(&out, "firewatch_http_errors_total %d\n", errors)
	out.WriteString(m.durations.text("firewatch_http_request_duration_seconds"))
	return out.String()
}

// statusClass maps an HTTP status code to its reporting class. A code outside the
// standard ranges is reported as "unknown" rather than dropped, so the classes
// always sum to every response the middleware recorded.
func statusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500:
		return "5xx"
	default:
		return "unknown"
	}
}

// histogram is a fixed-bucket cumulative histogram of seconds. Only the bucket
// counts are stored; every reading adds to the running sum and total, so the
// Prometheus sum/count pair is exact.
type histogram struct {
	buckets []float64 // upper bounds, ascending, excluding +Inf
	counts  []uint64  // counts[i] counts observations in (buckets[i-1], buckets[i]]
	sum     float64
	total   uint64
}

func newHistogram(bounds []float64) histogram {
	return histogram{buckets: bounds, counts: make([]uint64, len(bounds))}
}

// observe adds one observation. A value at or below a bound falls in that bucket;
// a value above every bound falls in the implicit +Inf bucket and is only counted
// in the total, which is what the +Inf line renders.
func (h *histogram) observe(value float64) {
	h.sum += value
	h.total++
	for i, bound := range h.buckets {
		if value <= bound {
			h.counts[i]++
			return
		}
	}
}

// text renders the Prometheus bucket/sum/count lines for one metric name. The
// buckets are rendered cumulatively, which is what the Prometheus text format
// requires and what histogram_quantile reads to compute a p95.
func (h *histogram) text(name string) string {
	var out strings.Builder
	var cumulative uint64
	for i, bound := range h.buckets {
		cumulative += h.counts[i]
		fmt.Fprintf(&out, "%s_bucket{le=\"%g\"} %d\n", name, bound, cumulative)
	}
	fmt.Fprintf(&out, "%s_bucket{le=\"+Inf\"} %d\n", name, h.total)
	fmt.Fprintf(&out, "%s_sum %g\n", name, h.sum)
	fmt.Fprintf(&out, "%s_count %d\n", name, h.total)
	return out.String()
}
