package limit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The three policies the gateway deploys, probed for the invariants that matter
// under load: concurrent calls never admit more than the burst, a denied call
// never manufactures a token, refill is exactly the configured rate, and a
// subject's exhaustion leaves another subject's quota untouched.
func TestConcurrentBudgetAndDeniedCallInvariant(t *testing.T) {
	now := time.Unix(100, 0)
	clock := func() time.Time { return now }

	for _, policy := range []Policy{{Rate: 20, Burst: 40}, {Rate: 5, Burst: 10}, {Rate: 1, Burst: 5}} {
		b := NewBucket(policy, clock)
		var accepted atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 200; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if b.Allow() {
					accepted.Add(1)
				}
			}()
		}
		wg.Wait()
		if accepted.Load() != int64(policy.Burst) {
			t.Fatalf("policy=%v accepted=%d want=%v", policy, accepted.Load(), policy.Burst)
		}
		for i := 0; i < 20; i++ {
			if b.Allow() {
				t.Fatal("a denied call manufactured a token")
			}
		}
		now = now.Add(time.Second)
		for i := 0; i < int(policy.Rate); i++ {
			if !b.Allow() {
				t.Fatal("one-second refill is missing a token")
			}
		}
		if b.Allow() {
			t.Fatal("one-second refill granted more than the configured rate")
		}
	}

	k := NewKeyed(KeyedOptions{
		Policy: Policy{Rate: 5, Burst: 10},
		Max:    3,
		Idle:   time.Minute,
	}, clock)
	for i := 0; i < 10; i++ {
		if !k.Allow("a") {
			t.Fatal("initial subject burst is missing")
		}
	}
	for i := 0; i < 100; i++ {
		if k.Allow("a") {
			t.Fatal("a denial manufactured a subject token")
		}
	}
	if !k.Allow("b") {
		t.Fatal("another subject consumed this subject's quota")
	}
	k.Allow("c")
	k.Allow("d")
	if k.Len() != 3 {
		t.Fatalf("tracked subjects=%d want 3", k.Len())
	}
	now = now.Add(2 * time.Minute)
	k.Allow("e")
	if k.Len() != 1 {
		t.Fatalf("tracked subjects after idle eviction=%d want 1", k.Len())
	}
}
