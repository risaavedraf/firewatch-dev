package handler

import (
	"context"
	"net/http"
	"time"
)

// StatusCheck evaluates operational dependencies, not end-to-end pipeline delivery.
type StatusCheck func(context.Context) Status

type Dependency struct {
	State string `json:"state"`
}

type DeadLetterStatus struct {
	State         string     `json:"state"`
	Depth         *int64     `json:"depth"`
	SampledAt     *time.Time `json:"sampled_at"`
	AgeSeconds    *float64   `json:"age_seconds"`
	MaxAgeSeconds *float64   `json:"max_age_seconds"`
	MaxDepth      *int64     `json:"max_depth"`
}

type Dependencies struct {
	Database   Dependency       `json:"database"`
	Broker     Dependency       `json:"broker"`
	DeadLetter DeadLetterStatus `json:"dead_letter"`
}

type Status struct {
	CheckedAt    time.Time    `json:"checked_at"`
	Ready        bool         `json:"ready"`
	Reason       string       `json:"reason"`
	Dependencies Dependencies `json:"dependencies"`
}

func statusHandler(check StatusCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := Status{CheckedAt: time.Now().UTC(), Reason: "status_unavailable", Dependencies: Dependencies{
			Database: Dependency{"unknown"}, Broker: Dependency{"unknown"}, DeadLetter: DeadLetterStatus{State: "unknown"},
		}}
		if check != nil {
			s = check(r.Context())
		}
		code := http.StatusServiceUnavailable
		if s.Ready {
			code = http.StatusOK
		}
		respond(w, code, s)
	}
}
