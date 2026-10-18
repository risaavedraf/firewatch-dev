package evidence

import (
	"strings"
	"testing"
	"time"
)

func TestValidReferenceAcceptsOnlyBoundedCredentialFreeHTTPURLs(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "https", raw: "https://example.org/informe/2026", want: true},
		{name: "http", raw: "http://example.org/acta", want: true},
		{name: "query and fragment", raw: "https://example.org/a?b=1#c", want: true},
		{name: "empty", raw: "", want: false},
		{name: "blank", raw: "   ", want: false},
		{name: "relative", raw: "/informe", want: false},
		{name: "ftp", raw: "ftp://example.org/a", want: false},
		{name: "javascript", raw: "javascript:alert(1)", want: false},
		{name: "no host", raw: "https:///a", want: false},
		{name: "credentials", raw: "https://user:secret@example.org/a", want: false},
		{name: "username only", raw: "https://user@example.org/a", want: false},
		{name: "too long", raw: "https://example.org/" + strings.Repeat("a", MaxReferenceBytes), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidReference(tc.raw); got != tc.want {
				t.Errorf("ValidReference(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestValidCriterionRequiresNonBlankBoundedText(t *testing.T) {
	if ValidCriterion("") || ValidCriterion("   ") {
		t.Error("a blank criterion must be refused")
	}
	if !ValidCriterion("Acta de la junta y fotografías") {
		t.Error("a normal criterion must be accepted")
	}
	if ValidCriterion(strings.Repeat("a", MaxCriterionRunes+1)) {
		t.Error("an over-long criterion must be refused")
	}
}

func TestSufficientRequiresASufficientReviewOfTheLatestSnapshot(t *testing.T) {
	insufficient := &Review{EvidenceID: 3, Decision: Insuficiente}
	stale := &Review{EvidenceID: 2, Decision: Suficiente}
	current := &Review{EvidenceID: 3, Decision: Suficiente}

	for _, tc := range []struct {
		name           string
		latestEvidence int64
		latestReview   *Review
		want           bool
	}{
		{name: "no evidence", latestEvidence: 0, latestReview: current, want: false},
		{name: "no review", latestEvidence: 3, latestReview: nil, want: false},
		{name: "insufficient", latestEvidence: 3, latestReview: insufficient, want: false},
		{name: "stale sufficient review", latestEvidence: 3, latestReview: stale, want: false},
		{name: "current sufficient review", latestEvidence: 3, latestReview: current, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sufficient(tc.latestEvidence, tc.latestReview); got != tc.want {
				t.Errorf("Sufficient(%d, %+v) = %v, want %v", tc.latestEvidence, tc.latestReview, got, tc.want)
			}
		})
	}
}

func TestCutoffIncludesTheLatestReviewInstant(t *testing.T) {
	created := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	evidenceAt := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	reviewedAt := time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC)

	if got := Cutoff(created, time.Time{}, time.Time{}); !got.Equal(created) {
		t.Errorf("cutoff without evidence/review = %v, want the creation instant", got)
	}
	if got := Cutoff(created, evidenceAt, time.Time{}); !got.Equal(evidenceAt) {
		t.Errorf("cutoff with evidence only = %v, want %v", got, evidenceAt)
	}
	// A batch published before the review must never masquerade as a reassessment
	// of the reviewed snapshot, so the review instant is the strongest lower bound.
	if got := Cutoff(created, evidenceAt, reviewedAt); !got.Equal(reviewedAt) {
		t.Errorf("cutoff with a review = %v, want %v", got, reviewedAt)
	}
}
