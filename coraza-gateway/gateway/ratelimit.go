package gateway

import (
	"fmt"
	"strings"
	"sync"
)

// RateLimitKind names one of the operator-facing frequency limits.
type RateLimitKind string

const (
	// RateLimitAccess counts every request from a client.
	RateLimitAccess RateLimitKind = "access"
	// RateLimitURL counts requests from a client to one specific URL.
	RateLimitURL RateLimitKind = "url"
	// RateLimitNotFound counts responses that came back 404, which is how
	// directory/file scanners look from the outside.
	RateLimitNotFound RateLimitKind = "notfound"
	// RateLimitAttack counts requests that actually matched a WAF rule.
	RateLimitAttack RateLimitKind = "attack"
)

const (
	maxRateLimitPeriodSec = 3600
	maxRateLimitBanSec    = 86400
	// maxTrackedRateKeys bounds the counter map. A flood from many unique
	// addresses must cost bounded memory, so the map is swept and, if that is not
	// enough, cleared — losing counts is acceptable, unbounded growth is not.
	maxTrackedRateKeys = 50000
)

// RateLimitConfig is one configured limit for one site.
//
// BanSec == 0 means "count and record, but do not ban": an explicitly
// detection-only posture for a limit, matching the engine's detection mode.
type RateLimitConfig struct {
	Kind      RateLimitKind `json:"kind"`
	PeriodSec int           `json:"periodSec"`
	Threshold int           `json:"threshold"`
	BanSec    int           `json:"banSec,omitempty"`
	// PerURL counts each request target separately. It is what distinguishes the
	// access limit's "URL mode" from its "global mode"; the dedicated URL limit
	// always counts per URL.
	PerURL bool `json:"perUrl,omitempty"`
}

func (c RateLimitConfig) validate() error {
	switch c.Kind {
	case RateLimitAccess, RateLimitURL, RateLimitNotFound, RateLimitAttack:
	default:
		return fmt.Errorf("unknown rate limit kind %q", c.Kind)
	}
	if c.PeriodSec < 1 || c.PeriodSec > maxRateLimitPeriodSec {
		return fmt.Errorf("rate limit %q period %d must be between 1 and %d seconds", c.Kind, c.PeriodSec, maxRateLimitPeriodSec)
	}
	if c.Threshold < 1 {
		return fmt.Errorf("rate limit %q threshold %d must be at least 1", c.Kind, c.Threshold)
	}
	if c.BanSec < 0 || c.BanSec > maxRateLimitBanSec {
		return fmt.Errorf("rate limit %q ban duration %d must be between 0 and %d seconds", c.Kind, c.BanSec, maxRateLimitBanSec)
	}
	return nil
}

// countsPerURL reports whether this limit tracks each target separately.
//
// The 404 limit is deliberately excluded: a scanner's signature is many misses
// across DIFFERENT paths, so bucketing it per target would hold every bucket
// below the threshold and the limit could never fire.
func (c RateLimitConfig) countsPerURL() bool {
	if c.Kind == RateLimitNotFound {
		return false
	}
	return c.PerURL || c.Kind == RateLimitURL
}

// slidingCounter is a fixed-memory sliding window: one bucket per second of the
// window, advanced lazily so a request costs O(seconds elapsed) rather than
// O(window). The running total makes the threshold check O(1).
type slidingCounter struct {
	period  int
	buckets []int32
	total   int
	lastSec int64
}

func newSlidingCounter(period int) *slidingCounter {
	return &slidingCounter{period: period, buckets: make([]int32, period)}
}

// add records one hit at the given unix second and returns the window total.
func (c *slidingCounter) add(now int64) int {
	if c.lastSec == 0 {
		c.lastSec = now
	}
	// A clock that jumps backwards must not rewind the window and let a client
	// replay its budget; hold the window where it is instead.
	if now < c.lastSec {
		now = c.lastSec
	}
	if now-c.lastSec >= int64(c.period) {
		for i := range c.buckets {
			c.buckets[i] = 0
		}
		c.total = 0
	} else {
		for s := c.lastSec + 1; s <= now; s++ {
			idx := int(s % int64(c.period))
			c.total -= int(c.buckets[idx])
			c.buckets[idx] = 0
		}
	}
	c.lastSec = now
	idx := int(now % int64(c.period))
	c.buckets[idx]++
	c.total++
	return c.total
}

// expired reports that the whole window has elapsed, so the entry holds no
// counts and can be dropped.
func (c *slidingCounter) expired(now int64) bool {
	return now-c.lastSec >= int64(c.period)
}

// rateLimiter holds every live sliding window, keyed by site+limit+client
// (+target for per-URL limits).
type rateLimiter struct {
	mu       sync.Mutex
	counters map[string]*slidingCounter
	capacity int
	// clearedOnce records that the map was dropped at least once because the
	// sweep could not free enough room. It is reported on /healthz rather than
	// hidden, so a flood that outruns the tracker is visible instead of silently
	// degrading enforcement.
	clearedOnce bool
}

func newRateLimiter(capacity int) *rateLimiter {
	if capacity <= 0 {
		capacity = maxTrackedRateKeys
	}
	return &rateLimiter{counters: make(map[string]*slidingCounter), capacity: capacity}
}

// hit records one event against key and returns the current window total.
func (l *rateLimiter) hit(key string, period int, now int64) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	if c, ok := l.counters[key]; ok {
		return c.add(now)
	}
	if len(l.counters) >= l.capacity {
		l.sweepLocked(now)
		if len(l.counters) >= l.capacity {
			l.counters = make(map[string]*slidingCounter)
			l.clearedOnce = true
		}
	}
	c := newSlidingCounter(period)
	l.counters[key] = c
	return c.add(now)
}

// sweepLocked drops entries whose window has fully elapsed; their totals are
// already zero, so nothing is lost.
func (l *rateLimiter) sweepLocked(now int64) {
	for k, c := range l.counters {
		if c.expired(now) {
			delete(l.counters, k)
		}
	}
}

func (l *rateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.counters)
}

func (l *rateLimiter) overflowed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.clearedOnce
}

// rateKey builds the counter key. Host scopes counts to a site so one site's
// traffic cannot exhaust another's budget.
func rateKey(host string, kind RateLimitKind, ip, target string) string {
	var b strings.Builder
	b.Grow(len(host) + len(kind) + len(ip) + len(target) + 3)
	b.WriteString(host)
	b.WriteByte('|')
	b.WriteString(string(kind))
	b.WriteByte('|')
	b.WriteString(ip)
	if target != "" {
		b.WriteByte('|')
		b.WriteString(target)
	}
	return b.String()
}
