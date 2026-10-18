package metrics

import (
	"fmt"
	"runtime"
	rmetrics "runtime/metrics"
	"strings"
)

// RuntimeText renders the process-resource signals: goroutines, CPU seconds and
// memory bytes. It is the infrastructure layer of /metrics at the granularity a
// Go process reports without a collector agent.
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
