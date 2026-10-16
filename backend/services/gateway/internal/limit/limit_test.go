package limit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually advanced Clock, so refill and eviction are proven
// without sleeping for minutes.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0).UTC()}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestBucketAllowsBurstThenDenies(t *testing.T) {
	clock := newFakeClock()
	bucket := NewBucket(Policy{Rate: 1, Burst: 3}, clock.Now)

	for i := 0; i < 3; i++ {
		if !bucket.Allow() {
			t.Fatalf("request %d within the burst was denied", i)
		}
	}
	if bucket.Allow() {
		t.Fatal("request beyond the burst was allowed")
	}

	clock.Advance(time.Second)
	if !bucket.Allow() {
		t.Fatal("a token refilled after one second was not available")
	}
}

func TestBucketRefillsOnlyUpToBurst(t *testing.T) {
	clock := newFakeClock()
	bucket := NewBucket(Policy{Rate: 1, Burst: 2}, clock.Now)

	if !bucket.Allow() || !bucket.Allow() {
		t.Fatal("the initial burst was not fully available")
	}
	// A long idle period must not accumulate more than Burst tokens.
	clock.Advance(time.Hour)
	if !bucket.Allow() || !bucket.Allow() {
		t.Fatal("a full hour of refill did not restore the burst")
	}
	if bucket.Allow() {
		t.Fatal("the bucket exceeded its burst after a long idle period")
	}
}

func TestKeyedIsolatesSubjects(t *testing.T) {
	clock := newFakeClock()
	keyed := NewKeyed(KeyedOptions{
		Policy: Policy{Rate: 5, Burst: 10},
		Max:    100,
		Idle:   time.Minute,
	}, clock.Now)

	for i := 0; i < 10; i++ {
		if !keyed.Allow("subject-a") {
			t.Fatalf("subject-a request %d within its burst was denied", i)
		}
	}
	if keyed.Allow("subject-a") {
		t.Fatal("subject-a exceeded its burst")
	}
	if !keyed.Allow("subject-b") {
		t.Fatal("subject-b's quota was consumed by subject-a")
	}
}

func TestKeyedEvictsIdleSubjects(t *testing.T) {
	clock := newFakeClock()
	keyed := NewKeyed(KeyedOptions{
		Policy: Policy{Rate: 5, Burst: 10},
		Max:    100,
		Idle:   time.Minute,
	}, clock.Now)

	if !keyed.Allow("subject-a") {
		t.Fatal("subject-a's first request was denied")
	}
	if got := keyed.Len(); got != 1 {
		t.Fatalf("tracked buckets after one subject = %d, want 1", got)
	}

	// Idle past the eviction threshold, then touch a different subject: the
	// sweep must have dropped the idle bucket.
	clock.Advance(2 * time.Minute)
	if !keyed.Allow("subject-b") {
		t.Fatal("subject-b's first request was denied")
	}
	if got := keyed.Len(); got != 1 {
		t.Fatalf("tracked buckets after eviction = %d, want 1", got)
	}
}

func TestKeyedHoldsHardCap(t *testing.T) {
	clock := newFakeClock()
	keyed := NewKeyed(KeyedOptions{
		Policy: Policy{Rate: 5, Burst: 10},
		Max:    3,
		Idle:   time.Hour,
	}, clock.Now)

	for i := 0; i < 50; i++ {
		keyed.Allow(fmt.Sprintf("subject-%d", i))
		if got := keyed.Len(); got > 3 {
			t.Fatalf("tracked buckets grew past the cap: %d > 3", got)
		}
	}
	if got := keyed.Len(); got != 3 {
		t.Fatalf("tracked buckets = %d, want the cap 3", got)
	}
}

// The limiter is reached from concurrent HTTP handlers, so it must stay
// race-free. Run under -race.
func TestLimiterIsSafeUnderConcurrency(t *testing.T) {
	clock := newFakeClock()
	bucket := NewBucket(Policy{Rate: 1000, Burst: 1000}, clock.Now)
	keyed := NewKeyed(KeyedOptions{
		Policy: Policy{Rate: 1000, Burst: 1000},
		Max:    32,
		Idle:   time.Hour,
	}, clock.Now)

	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				bucket.Allow()
				keyed.Allow(fmt.Sprintf("subject-%d", (g+i)%16))
				keyed.Len()
			}
		}(g)
	}
	wg.Wait()
}
