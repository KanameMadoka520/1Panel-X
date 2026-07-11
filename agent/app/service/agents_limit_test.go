package service

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestValidateAIAgentCapacityUnlimited(t *testing.T) {
	if err := validateAIAgentCapacity(100, 0); err != nil {
		t.Fatalf("expected zero limit to allow unlimited agents, got %v", err)
	}
}

func TestValidateAIAgentCapacityBelowLimit(t *testing.T) {
	if err := validateAIAgentCapacity(4, 5); err != nil {
		t.Fatalf("expected capacity below the limit, got %v", err)
	}
}

func TestValidateAIAgentCapacityReached(t *testing.T) {
	if err := validateAIAgentCapacity(5, 5); err == nil {
		t.Fatal("expected capacity at the configured limit to be rejected")
	}
}

func TestAgentInstallHooksSkipAppMetadataLimit(t *testing.T) {
	if !shouldCheckAppLimit(nil) {
		t.Fatal("expected regular app installs to enforce app store metadata limits")
	}
	if !shouldCheckAppLimit(&appInstallHooks{}) {
		t.Fatal("expected default hooks to enforce app store metadata limits")
	}
	if shouldCheckAppLimit(&appInstallHooks{SkipAppLimit: true}) {
		t.Fatal("expected AI agent installs to bypass app store metadata limits")
	}
}

func TestAIAgentLimiterUnlimited(t *testing.T) {
	l := &aiAgentLimiter{}
	calls := 0
	countFn := func() (int64, error) { calls++; return 999, nil }
	for i := 0; i < 3; i++ {
		release, err := l.reserve(0, countFn)
		if err != nil {
			t.Fatalf("unlimited reserve should never fail, got %v", err)
		}
		release()
	}
	if calls != 0 {
		t.Fatalf("unlimited limit must not query the count, called %d times", calls)
	}
	if l.inFlight != 0 {
		t.Fatalf("inFlight should be zero after unlimited reservations, got %d", l.inFlight)
	}
}

// TestAIAgentLimiterConcurrentDoesNotExceed hammers the limiter with far more
// concurrent creators than the limit and asserts the committed count can never
// exceed the limit, that exactly the limit is filled, and that inFlight drains.
func TestAIAgentLimiterConcurrentDoesNotExceed(t *testing.T) {
	const limit int64 = 3
	const goroutines = 32

	l := &aiAgentLimiter{}
	var committed int64
	countFn := func() (int64, error) { return atomic.LoadInt64(&committed), nil }

	var maxSeen int64
	var maxMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := l.reserve(limit, countFn)
			if err != nil {
				return // rejected: limit reached
			}
			// Commit happens while the reservation is still held (as in Create,
			// where agentRepo.Create precedes the deferred release).
			now := atomic.AddInt64(&committed, 1)
			maxMu.Lock()
			if now > maxSeen {
				maxSeen = now
			}
			maxMu.Unlock()
			release()
		}()
	}
	wg.Wait()

	if maxSeen > limit {
		t.Fatalf("committed agents exceeded the limit: saw %d, limit %d", maxSeen, limit)
	}
	if got := atomic.LoadInt64(&committed); got != limit {
		t.Fatalf("expected exactly %d committed agents, got %d", limit, got)
	}
	if l.inFlight != 0 {
		t.Fatalf("inFlight leaked: %d", l.inFlight)
	}
}

// TestAIAgentLimiterReleaseOnFailure verifies a reservation released without a
// commit (a failed install) frees its slot for a later creation.
func TestAIAgentLimiterReleaseOnFailure(t *testing.T) {
	const limit int64 = 1
	l := &aiAgentLimiter{}
	var committed int64
	countFn := func() (int64, error) { return atomic.LoadInt64(&committed), nil }

	// First reservation fails its install and releases without committing.
	release, err := l.reserve(limit, countFn)
	if err != nil {
		t.Fatalf("first reserve should succeed, got %v", err)
	}
	release() // no commit

	// The slot must be reusable.
	release2, err := l.reserve(limit, countFn)
	if err != nil {
		t.Fatalf("slot should be free after a released reservation, got %v", err)
	}
	atomic.AddInt64(&committed, 1)
	release2()

	// Now the single slot is committed; a new reservation must be rejected.
	if _, err := l.reserve(limit, countFn); err == nil {
		t.Fatal("expected reservation to be rejected once the limit is committed")
	}
	if l.inFlight != 0 {
		t.Fatalf("inFlight leaked: %d", l.inFlight)
	}
}
