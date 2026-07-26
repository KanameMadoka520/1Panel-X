package wafconfig

import (
	"fmt"
	"sort"
	"strings"
)

// Rate-limit kinds. These mirror the data plane's contract exactly; the gateway
// validates them again on load, so a mismatch fails closed rather than being
// silently dropped.
const (
	RateLimitAccess   = "access"
	RateLimitURL      = "url"
	RateLimitNotFound = "notfound"
	RateLimitAttack   = "attack"
)

const (
	MaxRateLimitPeriodSec = 3600
	MaxRateLimitBanSec    = 86400
)

// RateLimit is one frequency limit for one site.
//
// BanSec == 0 means the limit is recorded but does not ban — the same
// detection-only posture the engine offers, expressed per limit.
type RateLimit struct {
	Kind      string `json:"kind"`
	PeriodSec int    `json:"periodSec"`
	Threshold int    `json:"threshold"`
	BanSec    int    `json:"banSec,omitempty"`
	// PerURL counts each request target separately. It is what the access
	// limit's "URL mode" means; the dedicated URL limit always counts per target.
	PerURL bool `json:"perUrl,omitempty"`
}

// NormalizeRateLimits validates and canonically orders a set of limits.
//
// Ordering is deterministic so an unchanged policy always produces the same
// config-generation digest; an invalid or duplicated limit is a hard error
// surfaced to the operator rather than a silently dropped rule.
func NormalizeRateLimits(limits []RateLimit) ([]RateLimit, error) {
	if len(limits) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(limits))
	out := make([]RateLimit, 0, len(limits))
	for _, l := range limits {
		l.Kind = strings.TrimSpace(l.Kind)
		switch l.Kind {
		case RateLimitAccess, RateLimitURL, RateLimitNotFound, RateLimitAttack:
		default:
			return nil, fmt.Errorf("unknown rate limit kind %q", l.Kind)
		}
		if l.PeriodSec < 1 || l.PeriodSec > MaxRateLimitPeriodSec {
			return nil, fmt.Errorf("rate limit %q period %d must be between 1 and %d seconds", l.Kind, l.PeriodSec, MaxRateLimitPeriodSec)
		}
		if l.Threshold < 1 {
			return nil, fmt.Errorf("rate limit %q threshold %d must be at least 1", l.Kind, l.Threshold)
		}
		if l.BanSec < 0 || l.BanSec > MaxRateLimitBanSec {
			return nil, fmt.Errorf("rate limit %q ban duration %d must be between 0 and %d seconds", l.Kind, l.BanSec, MaxRateLimitBanSec)
		}
		// The dedicated URL limit is per-target by definition; normalizing it here
		// keeps the emitted config from carrying a contradictory flag.
		if l.Kind == RateLimitURL {
			l.PerURL = true
		}
		// The attack limit is counted from the engine's match callback, which has
		// no request target to key on, so a per-URL attack limit would be a flag
		// the data plane silently ignores.
		if l.Kind == RateLimitAttack {
			l.PerURL = false
		}
		if _, dup := seen[l.Kind]; dup {
			return nil, fmt.Errorf("duplicate rate limit kind %q", l.Kind)
		}
		seen[l.Kind] = struct{}{}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out, nil
}
