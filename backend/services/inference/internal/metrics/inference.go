package metrics

import (
	"fmt"
	"strings"
	"sync"

	"firewatch.local/inference/internal/artifact"
)

// allLevels is the canonical level vocabulary, reported even at zero so a
// dashboard series never appears or disappears with traffic.
var allLevels = []artifact.Level{artifact.LevelLow, artifact.LevelMedium, artifact.LevelHigh}

// allCodes is the refusal vocabulary the request surface documents, also
// reported at zero. A code missing from this list but observed at runtime is
// still rendered, so a new refusal path cannot become invisible.
var allCodes = []string{
	"invalid_body",
	"duplicate_feature",
	"missing_feature",
	"unknown_feature",
	"out_of_range",
	"non_finite_probability",
	"scoring_failed",
}

// Inference counts what the model layer produced: answers per level and
// refusals per stable code. The handler writes from many request goroutines
// while /metrics reads from one, so the counters are mutex-guarded.
//
// Refusals are counted by code rather than lumped into one error total because
// the codes are the contract: a spike in missing_feature is a caller that
// deployed a different feature set, while a spike in out_of_range is a unit
// problem in the data feed. One number could not tell those apart.
type Inference struct {
	mu          sync.Mutex
	predictions map[artifact.Level]uint64
	errors      map[string]uint64
}

// Prediction records one answered request.
func (m *Inference) Prediction(level artifact.Level) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.predictions == nil {
		m.predictions = map[artifact.Level]uint64{}
	}
	m.predictions[level]++
}

// Refused records one refused request under its stable code.
func (m *Inference) Refused(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errors == nil {
		m.errors = map[string]uint64{}
	}
	m.errors[code]++
}

// Text renders the model layer: one labeled series per level and per refusal
// code, plus the total each grouping sums to.
func (m *Inference) Text() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out strings.Builder
	var total uint64
	for _, level := range allLevels {
		value := m.predictions[level]
		total += value
		fmt.Fprintf(&out, "firewatch_inference_predictions_total{level=%q} %d\n", level, value)
	}
	fmt.Fprintf(&out, "firewatch_inference_answers_total %d\n", total)
	var refused uint64
	rendered := make(map[string]bool, len(allCodes))
	for _, code := range allCodes {
		rendered[code] = true
		value := m.errors[code]
		refused += value
		fmt.Fprintf(&out, "firewatch_inference_refusals_total{code=%q} %d\n", code, value)
	}
	// Codes observed at runtime but not in the documented list are appended in
	// a stable order, so the labeled series still sums to every refusal.
	extra := make([]string, 0, len(m.errors))
	for code := range m.errors {
		if !rendered[code] {
			extra = append(extra, code)
		}
	}
	sortStrings(extra)
	for _, code := range extra {
		value := m.errors[code]
		refused += value
		fmt.Fprintf(&out, "firewatch_inference_refusals_total{code=%q} %d\n", code, value)
	}
	fmt.Fprintf(&out, "firewatch_inference_refused_total %d\n", refused)
	return out.String()
}

// ModelText renders the active model as an info series: version and data_hash
// as labels, value 1. A dashboard can then join a probability it is looking at
// to the exact artifact that produced it, which is the question an operator
// actually asks after a model swap.
func ModelText(version, dataHash string) string {
	return fmt.Sprintf("firewatch_inference_model_info{version=%q,data_hash=%q} 1\n", version, dataHash)
}

// sortStrings is an insertion sort over a handful of codes. Importing sort for
// at most one new code per release would be fine too; keeping this package
// dependency-free is the point of the file.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
