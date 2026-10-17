// Package metrics holds the risk service's own observability counters: the
// outcomes of the model, which /metrics exposes alongside the broker's delivery
// counters. They are kept separate from messaging.MetricsText because they
// measure model output, not message delivery.
package metrics

import (
	"fmt"
	"sync/atomic"

	"firewatch.local/risk/internal/classify"
)

// RiskOutcomes counts published assessments per canonical risk level. The
// publication path writes from the consumer goroutine while /metrics reads from
// an HTTP goroutine, so every field is atomic and the type is safe for
// concurrent use.
//
// A level that is not one of the three severity levels is counted under
// "desconocido" rather than dropped. The classifier's fourth level is exactly
// that value, and counting it there keeps the labeled series summing to the
// number of observed assessments: a level that silently vanished from the
// distribution would be invisible, which is the opposite of what an
// observability counter is for.
type RiskOutcomes struct {
	bajo        atomic.Uint64
	medio       atomic.Uint64
	alto        atomic.Uint64
	desconocido atomic.Uint64
}

// Observe records one published assessment at the given level.
func (o *RiskOutcomes) Observe(level classify.Level) {
	switch level {
	case classify.LevelBajo:
		o.bajo.Add(1)
	case classify.LevelMedio:
		o.medio.Add(1)
	case classify.LevelAlto:
		o.alto.Add(1)
	default:
		o.desconocido.Add(1)
	}
}

// HighRisk reports how many assessments reached the Alto level.
func (o *RiskOutcomes) HighRisk() uint64 { return o.alto.Load() }

// Text renders the Prometheus text lines for the model-outcome counters. The
// high-risk total keeps the exact metric name the observability plan requires
// (high_risk_predictions_total, the counter the Session 4.2 activity asks to
// complete); the distribution is one labeled series per canonical level so a
// dashboard can read Bajo/Medio/Alto directly without deriving them.
func (o *RiskOutcomes) Text() string {
	return fmt.Sprintf(
		"firewatch_high_risk_predictions_total %d\n"+
			"firewatch_risk_predictions_total{level=\"bajo\"} %d\n"+
			"firewatch_risk_predictions_total{level=\"medio\"} %d\n"+
			"firewatch_risk_predictions_total{level=\"alto\"} %d\n"+
			"firewatch_risk_predictions_total{level=\"desconocido\"} %d\n",
		o.alto.Load(), o.bajo.Load(), o.medio.Load(), o.alto.Load(), o.desconocido.Load())
}
