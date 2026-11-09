package metrics

import (
	"strconv"
	"strings"
	"testing"
)

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
