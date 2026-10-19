// Package evidence holds the pure rules of the preventive evidence workflow: what
// counts as a bounded, credential-free reference, whether a human decision makes
// the current evidence snapshot sufficient, and the instant a later synthetic
// batch must not precede.
//
// It has no storage, HTTP or clock dependency so the repository can re-evaluate
// every rule inside the transaction that owns the row, and the transport can
// reject malformed input before storage is touched.
package evidence

import (
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// Decision is one human sufficiency decision. The literals are the wire values.
type Decision string

const (
	Suficiente   Decision = "suficiente"
	Insuficiente Decision = "insuficiente"
)

// Bounds keep one evidence snapshot small and one request bounded.
const (
	MaxCriterionRunes = 2000
	MaxReferenceBytes = 2048
	MaxReferences     = 10
)

// ValidDecision reports whether value is a decision the workflow defines.
func ValidDecision(value string) bool {
	return Decision(value) == Suficiente || Decision(value) == Insuficiente
}

// ValidCriterion reports whether raw is a non-blank, bounded criterion.
func ValidCriterion(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	return utf8.RuneCountInString(raw) <= MaxCriterionRunes
}

// ValidReference reports whether raw is an http(s) URL with a host, without
// embedded credentials, bounded in length. The URL is parsed for shape only and
// never fetched: a reference is a citation, not a remote retrieval instruction.
func ValidReference(raw string) bool {
	if raw == "" || len(raw) > MaxReferenceBytes || strings.TrimSpace(raw) != raw {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return false
	}
	// Userinfo is how a URL carries a credential. Evidence must never embed one.
	if parsed.User != nil {
		return false
	}
	return true
}

// Review is the minimal view of the latest human decision.
type Review struct {
	EvidenceID int64
	Decision   Decision
}

// Sufficient reports whether the current evidence snapshot carries a sufficient
// human review. A review of an older snapshot is stale and never counts, which is
// what stops an acceptance from reusing a decision made against replaced evidence.
func Sufficient(latestEvidenceID int64, latest *Review) bool {
	if latestEvidenceID <= 0 || latest == nil {
		return false
	}
	return latest.EvidenceID == latestEvidenceID && latest.Decision == Suficiente
}

// Cutoff returns the instant a reassessment batch must not precede: the latest of
// the action's creation, the latest evidence snapshot and the latest human review.
// A zero instant is ignored, so an action without evidence uses its creation and
// an evidence snapshot without a review uses the snapshot's instant.
func Cutoff(actionCreatedAt, latestEvidenceAt, latestReviewedAt time.Time) time.Time {
	cutoff := actionCreatedAt
	for _, candidate := range []time.Time{latestEvidenceAt, latestReviewedAt} {
		if !candidate.IsZero() && candidate.After(cutoff) {
			cutoff = candidate
		}
	}
	return cutoff
}
