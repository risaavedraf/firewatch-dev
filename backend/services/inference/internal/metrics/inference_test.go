package metrics

import (
	"strings"
	"testing"

	"firewatch.local/inference/internal/artifact"
)

// TestInferenceCountsEveryLevelAndRefusal protects the wire format of the model
// layer: one series per level and per documented code, rendered even at zero so a
// dashboard never loses a series, plus the two totals that prove the labels sum
// to every observation.
func TestInferenceCountsEveryLevelAndRefusal(t *testing.T) {
	counters := &Inference{}
	counters.Prediction(artifact.LevelLow)
	counters.Prediction(artifact.LevelHigh)
	counters.Prediction(artifact.LevelHigh)
	counters.Refused("out_of_range")
	counters.Refused("missing_feature")

	text := counters.Text()
	for _, want := range []string{
		`firewatch_inference_predictions_total{level="low"} 1`,
		`firewatch_inference_predictions_total{level="medium"} 0`,
		`firewatch_inference_predictions_total{level="high"} 2`,
		`firewatch_inference_answers_total 3`,
		`firewatch_inference_refusals_total{code="out_of_range"} 1`,
		`firewatch_inference_refusals_total{code="missing_feature"} 1`,
		`firewatch_inference_refusals_total{code="unknown_feature"} 0`,
		`firewatch_inference_refusals_total{code="non_finite_probability"} 0`,
		`firewatch_inference_refused_total 2`,
	} {
		if !strings.Contains(text, want+"\n") {
			t.Errorf("metrics text is missing %q:\n%s", want, text)
		}
	}
}

// TestInferenceRendersUndocumentedCodes keeps a new refusal path from becoming
// invisible: a code observed at runtime but absent from the documented list is
// appended in a stable order, and the refusal total still sums every label.
func TestInferenceRendersUndocumentedCodes(t *testing.T) {
	counters := &Inference{}
	counters.Refused("zeta_code")
	counters.Refused("alpha_code")
	counters.Refused("out_of_range")

	text := counters.Text()
	alpha := strings.Index(text, `firewatch_inference_refusals_total{code="alpha_code"} 1`)
	zeta := strings.Index(text, `firewatch_inference_refusals_total{code="zeta_code"} 1`)
	if alpha < 0 || zeta < 0 {
		t.Fatalf("undocumented codes are not rendered:\n%s", text)
	}
	if alpha > zeta {
		t.Errorf("undocumented codes are not rendered in a stable order:\n%s", text)
	}
	if !strings.Contains(text, "firewatch_inference_refused_total 3\n") {
		t.Errorf("the refusal total does not sum every label:\n%s", text)
	}
}

// TestModelTextNamesVersionAndDataHash pins the info series: an operator looking
// at a probability has to be able to join it to the artifact that produced it.
func TestModelTextNamesVersionAndDataHash(t *testing.T) {
	text := ModelText("valparaiso-logistic-v1", "9f2c41d7")
	want := `firewatch_inference_model_info{version="valparaiso-logistic-v1",data_hash="9f2c41d7"} 1` + "\n"
	if text != want {
		t.Fatalf("ModelText() = %q want %q", text, want)
	}
}
