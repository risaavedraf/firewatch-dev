package scorer

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// equivalenceTolerance is the contract between the Go scorer and the frozen
// reference column: 1e-9 is roughly seven significant digits, which is far
// tighter than any modelling difference and loose enough that two independent
// IEEE-754 evaluations of the same formula agree.
const equivalenceTolerance = 1e-9

// TestReferenceScorerEquivalence replays the frozen fixture produced by
// tools/reference_scorer.py (pure stdlib, no numpy) through the Go scorer. The
// script is not run in CI: the CSV is the fixture, and the script documents where
// it came from. If the model or the pipeline changes, the CSV is regenerated
// deliberately and its diff is reviewed, so a change in the Go scorer can never
// silently redefine the expected numbers too.
func TestReferenceScorerEquivalence(t *testing.T) {
	model := load(t, "logistic.json")
	scorer := New(model)

	file, err := os.Open(filepath.Join("..", "..", "testdata", "equivalence", "expected.csv"))
	if err != nil {
		t.Fatalf("open the frozen fixture: %v", err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read the frozen fixture: %v", err)
	}
	if len(rows) < 2 {
		t.Fatal("the frozen fixture has no data rows")
	}

	// The header must repeat the artifact's feature order exactly, and the
	// probability column must come last. A reordered header would silently
	// compare column i of the CSV against feature i of the model, which is the
	// one thing this fixture exists to check.
	header := rows[0]
	names := model.FeatureNames()
	if len(header) != len(names)+1 {
		t.Fatalf("header has %d columns for %d features plus probability", len(header), len(names))
	}
	for i, name := range names {
		if header[i] != name {
			t.Fatalf("header column %d is %q, the artifact order is %q", i, header[i], name)
		}
	}
	if header[len(names)] != "probability" {
		t.Fatalf("last header column is %q, want probability", header[len(names)])
	}

	for number, row := range rows[1:] {
		line := number + 2 // header is line 1
		if len(row) != len(header) {
			t.Fatalf("line %d has %d columns, want %d", line, len(row), len(header))
		}
		sample := make(Sample, 0, len(names))
		for i, name := range names {
			value, err := strconv.ParseFloat(row[i], 64)
			if err != nil {
				t.Fatalf("line %d column %q (%q) is not a number: %v", line, name, row[i], err)
			}
			sample = append(sample, FeatureValue{Name: name, Value: value})
		}
		expected, err := strconv.ParseFloat(row[len(names)], 64)
		if err != nil {
			t.Fatalf("line %d frozen probability %q is not a number: %v", line, row[len(names)], err)
		}
		result, err := scorer.Score(sample)
		if err != nil {
			t.Fatalf("line %d: score: %v", line, err)
		}
		if difference := math.Abs(result.Probability - expected); difference > equivalenceTolerance {
			t.Fatalf("line %d: Go scored %v, the reference scored %v (difference %v > %v)",
				line, result.Probability, expected, difference, equivalenceTolerance)
		}
	}
}
