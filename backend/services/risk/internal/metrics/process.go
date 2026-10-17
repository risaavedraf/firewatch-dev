package metrics

import (
	"fmt"
	"runtime"
	rmetrics "runtime/metrics"
	"strings"
)

// RuntimeText renders the process-resource signals: goroutines, CPU seconds and
// memory bytes. It is the infrastructure layer of /metrics at the granularity a
// Go process reports without a collector agent. GPU is absent on purpose: the
// model is tabular, runs on CPU and the sizing document states no GPU/VRAM is
// required.
//
// The samples are allocated per call on purpose. runtime/metrics.Read writes into
// the slice it is given, and /metrics may be scraped concurrently, so one shared
// slice would be a data race. Each value is read only when its kind matches, so a
// future runtime that renames or retypes a metric degrades to zero instead of
// panicking the scrape.
func RuntimeText() string {
	samples := []rmetrics.Sample{
		{Name: "/cpu/classes/total:cpu-seconds"},
		{Name: "/memory/classes/total:bytes"},
	}
	rmetrics.Read(samples)
	var cpu float64
	if samples[0].Value.Kind() == rmetrics.KindFloat64 {
		cpu = samples[0].Value.Float64()
	}
	var memory uint64
	if samples[1].Value.Kind() == rmetrics.KindUint64 {
		memory = samples[1].Value.Uint64()
	}
	var out strings.Builder
	fmt.Fprintf(&out, "firewatch_process_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(&out, "firewatch_process_cpu_seconds_total %g\n", cpu)
	fmt.Fprintf(&out, "firewatch_process_memory_bytes %d\n", memory)
	return out.String()
}

// PoolStats is a caller-captured snapshot of a database connection pool, so this
// package keeps no database dependency. The pool is the concrete signal for the
// "data access (PostGIS)" priority: a pool that is exhausted, or that never
// releases a connection, shows up here before it shows up as request latency.
type PoolStats struct {
	Total    int32
	Acquired int32
	Idle     int32
	Max      int32
}

// Text renders the pool gauges.
func (p PoolStats) Text() string {
	return fmt.Sprintf(
		"firewatch_db_pool_total_conns %d\n"+
			"firewatch_db_pool_acquired_conns %d\n"+
			"firewatch_db_pool_idle_conns %d\n"+
			"firewatch_db_pool_max_conns %d\n",
		p.Total, p.Acquired, p.Idle, p.Max)
}
