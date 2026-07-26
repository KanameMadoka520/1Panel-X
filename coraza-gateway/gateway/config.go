package gateway

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// SiteConfig maps one protected host to its origin upstream. The agent generates
// this from the managed-website registry (Phase 21); the gateway loads it to
// route each request to the right origin by Host.
type SiteConfig struct {
	WebsiteID uint   `json:"websiteId,omitempty"`
	Alias     string `json:"alias,omitempty"`
	Host      string `json:"host"`
	Upstream  string `json:"upstream"`
	Mode      Mode   `json:"mode,omitempty"`
	// AllowIPs are trusted client addresses/CIDRs that bypass CRS inspection but
	// are still proxied. DenyIPs are refused with a 403 before inspection. Both
	// are explicit operator ACLs honored regardless of Mode; deny wins.
	AllowIPs []string `json:"allowIps,omitempty"`
	DenyIPs  []string `json:"denyIps,omitempty"`
	// RateLimits are this site's frequency limits. They are evaluated after the
	// IP ACL, so an allow-listed client is never rate-limited.
	RateLimits []RateLimitConfig `json:"rateLimits,omitempty"`
}

// Config is the gateway's versioned per-site routing table.
type Config struct {
	Version    int          `json:"version,omitempty"`
	Generation string       `json:"generation,omitempty"`
	Sites      []SiteConfig `json:"sites"`
	// AttackRateLimit is gateway-wide, not per-site. A rule match is only
	// observable in detection mode through the engine's match callback, which
	// reports the client and the rule but not the Host, and one compiled engine
	// serves every site sharing a policy — so a per-site threshold here could not
	// actually be honoured. It is therefore modelled, and presented, as one
	// gateway-level limit.
	AttackRateLimit *RateLimitConfig `json:"attackRateLimit,omitempty"`
	// Lists are the panel-wide black/white lists (IP, IP group, URL, User-Agent).
	// They apply to every protected site; a site's own IP list is an additional,
	// narrower control layered underneath them.
	Lists []ListRule `json:"lists,omitempty"`
	// IPGroups are named address sets referenced by ipgroup list entries.
	IPGroups []IPGroup `json:"ipGroups,omitempty"`
}

// ConfigVersion is the contract version this build understands.
//
// A version NEWER than this one is refused rather than parsed leniently.
// encoding/json silently drops unknown fields, so an older gateway handed a
// newer config would come up enforcing a policy with the new fields missing
// while still echoing the new generation on /healthz — and the control plane,
// which confirms application by comparing generations, would report the policy
// as live. Refusing to start instead makes the container unhealthy, the
// readiness wait time out, and the control-plane transaction roll back honestly.
//
// Version 0 (absent) is still accepted so a hand-written config keeps working.
const ConfigVersion = 2

// LoadConfig reads and validates the routing config from a file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(data)
}

// ParseConfig validates the routing config: every site needs a non-empty,
// canonical host and an HTTP(S) origin URL. Hosts are normalized exactly as
// Router does, and duplicate normalized hosts are rejected instead of allowing
// the last entry to silently select a different origin (W12).
func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("waf config: invalid json: %w", err)
	}
	seen := make(map[string]struct{}, len(c.Sites))
	if c.Version < 0 || c.Version > ConfigVersion {
		return Config{}, fmt.Errorf("waf config: unsupported version %d (this gateway understands up to %d; upgrade the WAF gateway image)", c.Version, ConfigVersion)
	}
	if c.AttackRateLimit != nil {
		if c.AttackRateLimit.Kind == "" {
			c.AttackRateLimit.Kind = RateLimitAttack
		}
		if c.AttackRateLimit.Kind != RateLimitAttack {
			return Config{}, fmt.Errorf("waf config: attackRateLimit must be of kind %q, got %q", RateLimitAttack, c.AttackRateLimit.Kind)
		}
		if err := c.AttackRateLimit.validate(); err != nil {
			return Config{}, fmt.Errorf("waf config: %w", err)
		}
	}
	// Compiled here purely to reject an unusable list set at load time; the
	// router compiles it again for real. A bad entry must fail the whole config
	// rather than leave part of the operator's policy silently unenforced.
	if _, err := newListMatcher(c.Lists, c.IPGroups); err != nil {
		return Config{}, fmt.Errorf("waf config: %w", err)
	}
	for i := range c.Sites {
		host := normalizeHost(c.Sites[i].Host)
		if host == "" {
			return Config{}, fmt.Errorf("waf config: site %d has empty or invalid host", i)
		}
		if _, ok := seen[host]; ok {
			return Config{}, fmt.Errorf("waf config: duplicate normalized host %q", host)
		}
		seen[host] = struct{}{}
		c.Sites[i].Host = host
		if c.Sites[i].Mode != "" && c.Sites[i].Mode != ModeBlock && c.Sites[i].Mode != ModeDetection {
			return Config{}, fmt.Errorf("waf config: site %q has invalid mode %q", host, c.Sites[i].Mode)
		}

		u, err := url.Parse(c.Sites[i].Upstream)
		if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Fragment != "" ||
			(u.Scheme != "http" && u.Scheme != "https") {
			return Config{}, fmt.Errorf("waf config: site %q has invalid HTTP(S) upstream %q", host, c.Sites[i].Upstream)
		}
		if strings.TrimSpace(u.Hostname()) == "" {
			return Config{}, fmt.Errorf("waf config: site %q has invalid HTTP(S) upstream %q", host, c.Sites[i].Upstream)
		}
		if _, err := parseIPNets(c.Sites[i].AllowIPs); err != nil {
			return Config{}, fmt.Errorf("waf config: site %q allow %w", host, err)
		}
		if _, err := parseIPNets(c.Sites[i].DenyIPs); err != nil {
			return Config{}, fmt.Errorf("waf config: site %q deny %w", host, err)
		}
		seenKinds := make(map[RateLimitKind]struct{}, len(c.Sites[i].RateLimits))
		for _, rl := range c.Sites[i].RateLimits {
			if err := rl.validate(); err != nil {
				return Config{}, fmt.Errorf("waf config: site %q %w", host, err)
			}
			// The attack limit is gateway-wide; accepting a per-site copy here
			// would advertise a threshold the data plane cannot honour.
			if rl.Kind == RateLimitAttack {
				return Config{}, fmt.Errorf("waf config: site %q cannot set an attack rate limit; it is configured gateway-wide via attackRateLimit", host)
			}
			// Two limits of the same kind would share a counter key and silently
			// enforce whichever threshold happened to be checked first.
			if _, dup := seenKinds[rl.Kind]; dup {
				return Config{}, fmt.Errorf("waf config: site %q has duplicate rate limit kind %q", host, rl.Kind)
			}
			seenKinds[rl.Kind] = struct{}{}
		}
	}
	return c, nil
}
