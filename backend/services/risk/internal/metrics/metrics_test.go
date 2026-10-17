package metrics

import (
	"strings"
	"testing"

	"firewatch.local/risk/internal/classify"
)

// TestRiskOutcomesCountsEachLevel protects the counter wiring: every canonical
// level is counted in its own series and the named high-risk total tracks the
// Alto series, so a dashboard can read both without deriving one from the other.
func TestRiskOutcomesCountsEachLevel(t *testing.T) {
	outcomes := &RiskOutcomes{}
	for _, level := range []classify.Level{classify.LevelBajo, classify.LevelMedio, classify.LevelAlto, classify.LevelAlto} {
		outcomes.Observe(level)
	}
	outcomes.Observe(classify.LevelDesconocido)
	if got := outcomes.HighRisk(); got != 2 {
		t.Fatalf("high risk is %d, want 2", got)
	}
	text := outcomes.Text()
	for _, want := range []string{
		"firewatch_high_risk_predictions_total 2\n",
		"firewatch_risk_predictions_total{level=\"bajo\"} 1\n",
		"firewatch_risk_predictions_total{level=\"medio\"} 1\n",
		"firewatch_risk_predictions_total{level=\"alto\"} 2\n",
		"firewatch_risk_predictions_total{level=\"desconocido\"} 1\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics text %q does not contain %q", text, want)
		}
	}
}

// TestRiskOutcomesCountsAnUnknownLevelAsDesconocido protects the conservation
// rule: a level that is not one of the canonical severity levels is counted, not
// dropped, so the labeled series can never silently lose an observation, and it
// is never allowed to inflate the high-risk total.
func TestRiskOutcomesCountsAnUnknownLevelAsDesconocido(t *testing.T) {
	outcomes := &RiskOutcomes{}
	outcomes.Observe(classify.Level("fuera-de-catalogo"))
	if !strings.Contains(outcomes.Text(), "firewatch_risk_predictions_total{level=\"desconocido\"} 1\n") {
		t.Fatalf("an unknown level vanished from the distribution: %q", outcomes.Text())
	}
	if outcomes.HighRisk() != 0 {
		t.Fatalf("an unknown level counted as high risk: %d", outcomes.HighRisk())
	}
}
