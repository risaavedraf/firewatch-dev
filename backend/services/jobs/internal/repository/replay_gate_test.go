package repository

import (
	"errors"
	"strconv"
	"testing"
)

// TestReplayGatePinsTheRecoveryVocabulary pins the policy the guarded UPDATE
// enforces: a failed job is always replayable, a queued job only with the
// operator's explicit confirmation, and a published job never. The states are the
// migration 004 vocabulary, and a state the schema does not define fails closed
// instead of opening a recovery path nobody agreed to.
//
// It is the pure seam of the rule; the transaction, the SQL and the persisted
// rows are covered by TestReplayRestoresFailedJobAndEnqueuesOnce and
// TestReplayQueuedJobRequiresConfirmation, which need PostgreSQL.
func TestReplayGatePinsTheRecoveryVocabulary(t *testing.T) {
	for _, tc := range []struct {
		state         string
		confirmQueued bool
		want          error
	}{
		{state: "failed", want: nil},
		{state: "failed", confirmQueued: true, want: nil},
		{state: "queued", want: ErrReplayConfirmationRequired},
		{state: "queued", confirmQueued: true, want: nil},
		{state: "published", want: ErrJobNotReplayable},
		{state: "published", confirmQueued: true, want: ErrJobNotReplayable},
		{state: "", want: ErrJobNotReplayable},
		{state: "replayed", want: ErrJobNotReplayable},
	} {
		t.Run(tc.state+"_confirm="+strconv.FormatBool(tc.confirmQueued), func(t *testing.T) {
			if err := replayGate(tc.state, tc.confirmQueued); !errors.Is(err, tc.want) {
				t.Fatalf("replayGate(%q, %v)=%v want=%v", tc.state, tc.confirmQueued, err, tc.want)
			}
		})
	}
}
