package messaging

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
)

// The agreed retry budget is five attempts with literal 1, 2, 4 and 8 second
// waits, and a permanent error spends exactly one attempt and waits for nothing.
// The schedule is written out rather than compared against retryBackoff, so the
// test fails if the implementation and the agreed policy ever drift apart.
func TestRetryBudgetMatchesAgreedLiteralSchedule(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		calls := 0
		var sleeps []time.Duration
		attempts, err := handleWithRetries(
			context.Background(),
			func(context.Context, v1.Event) error {
				calls++
				if permanent {
					return testPermanentError{}
				}
				return errors.New("temporary")
			},
			func(_ context.Context, delay time.Duration) bool {
				sleeps = append(sleeps, delay)
				return true
			},
			v1.Event{},
		)
		want, expected := 5, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
		if permanent {
			want, expected = 1, nil
		}
		if err == nil || calls != want || attempts != want || !reflect.DeepEqual(sleeps, expected) {
			t.Fatalf("permanent=%v calls=%d attempts=%d sleeps=%v err=%v", permanent, calls, attempts, sleeps, err)
		}
	}
}
