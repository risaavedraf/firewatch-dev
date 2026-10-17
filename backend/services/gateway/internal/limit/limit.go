// Package limit provides the token-bucket quotas the gateway applies to
// inbound requests. It is deliberately small and clock-injectable so refill
// and state eviction are deterministic under test.
package limit

import (
	"sync"
	"time"
)

// Clock reports the current time. Injecting it lets tests drive refill and
// eviction without sleeping.
type Clock func() time.Time

// SystemClock is the production Clock.
func SystemClock() time.Time { return time.Now() }

// Policy is one token-bucket policy: at most Burst tokens, refilled at Rate
// tokens per second.
type Policy struct {
	Rate  float64
	Burst float64
}

// refill adds the tokens accrued over elapsed and never exceeds Burst.
func (p Policy) refill(tokens float64, elapsed time.Duration) float64 {
	return min(p.Burst, tokens+elapsed.Seconds()*p.Rate)
}

// Bucket is a single unkeyed token bucket. It is safe for concurrent use.
type Bucket struct {
	mu     sync.Mutex
	policy Policy
	clock  Clock
	tokens float64
	last   time.Time
}

// NewBucket returns a full token bucket governed by policy that reads the time
// from clock.
func NewBucket(policy Policy, clock Clock) *Bucket {
	return &Bucket{policy: policy, clock: clock, tokens: policy.Burst, last: clock()}
}

// Allow reports whether a token was available and consumes it. The bucket
// starts full and refills up to Burst.
func (b *Bucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock()
	b.tokens = b.policy.refill(b.tokens, now.Sub(b.last))
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// KeyedOptions configures a Keyed limiter.
type KeyedOptions struct {
	Policy Policy
	// Max is the hard cap on tracked keys. A new key while at the cap evicts
	// the least recently used entry, so an unbounded stream of distinct keys
	// cannot grow memory without bound.
	Max int
	// Idle is how long an unused key may stay before it is evicted.
	Idle time.Duration
}

// Keyed enforces one Policy per key with bounded, evictable state. It is safe
// for concurrent use.
type Keyed struct {
	mu      sync.Mutex
	opts    KeyedOptions
	clock   Clock
	buckets map[string]*bucket
}

// bucket is one key's token state.
type bucket struct {
	tokens float64
	last   time.Time
}

// NewKeyed returns an empty Keyed limiter.
func NewKeyed(opts KeyedOptions, clock Clock) *Keyed {
	return &Keyed{
		opts:    opts,
		clock:   clock,
		buckets: map[string]*bucket{},
	}
}

// Allow reports whether key has a token available and consumes it, creating a
// full bucket on first use.
func (k *Keyed) Allow(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.clock()
	k.evictIdle(now)
	b, ok := k.buckets[key]
	if !ok {
		if k.opts.Max > 0 && len(k.buckets) >= k.opts.Max {
			k.evictOldest()
		}
		b = &bucket{tokens: k.opts.Policy.Burst, last: now}
		k.buckets[key] = b
	}
	b.tokens = k.opts.Policy.refill(b.tokens, now.Sub(b.last))
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Len reports how many keys are tracked. It exists so the cap and eviction
// behavior can be asserted from tests.
func (k *Keyed) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.buckets)
}

// evictIdle removes every key untouched for longer than Idle.
func (k *Keyed) evictIdle(now time.Time) {
	for key, b := range k.buckets {
		if now.Sub(b.last) > k.opts.Idle {
			delete(k.buckets, key)
		}
	}
}

// evictOldest removes the least recently used key, keeping the cap hard.
func (k *Keyed) evictOldest() {
	var oldestKey string
	var oldest time.Time
	found := false
	for key, b := range k.buckets {
		if !found || b.last.Before(oldest) {
			oldestKey, oldest, found = key, b.last, true
		}
	}
	if found {
		delete(k.buckets, oldestKey)
	}
}
