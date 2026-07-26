package gateway

import (
	"net/http"
	"sort"
	"sync"
	"time"
)

// maxTrackedBans bounds the live ban table.
const maxTrackedBans = 20000

// BanEntry is one temporary block on a client address.
//
// Bans are keyed by client IP and are therefore gateway-wide, not per-site: a
// scanner sweeping every hosted site is one offender, and banning it once is
// both cheaper and closer to what an operator means. Host/WebsiteID record
// which site's limit triggered it.
type BanEntry struct {
	IP        string        `json:"ip"`
	Kind      RateLimitKind `json:"kind"`
	Host      string        `json:"host,omitempty"`
	WebsiteID uint          `json:"websiteId,omitempty"`
	BannedAt  time.Time     `json:"bannedAt"`
	ExpiresAt time.Time     `json:"expiresAt"`
}

type banStore struct {
	mu   sync.Mutex
	bans map[string]BanEntry
	max  int
}

func newBanStore(max int) *banStore {
	if max <= 0 {
		max = maxTrackedBans
	}
	return &banStore{bans: make(map[string]BanEntry), max: max}
}

// active returns the ban in force for ip, dropping it once it has expired.
//
// Expiry is lazy and emits no event: the ban record already carries ExpiresAt,
// so the control plane derives "expired" from the clock instead of waiting for
// a notification that would never arrive for an offender that never returns.
func (s *banStore) active(ip string, now time.Time) (BanEntry, bool) {
	if ip == "" {
		return BanEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.bans[ip]
	if !ok {
		return BanEntry{}, false
	}
	if !now.Before(entry.ExpiresAt) {
		delete(s.bans, ip)
		return BanEntry{}, false
	}
	return entry, true
}

// add installs or extends a ban. The longer expiry wins, so a second offence
// cannot shorten an existing ban.
func (s *banStore) add(entry BanEntry) BanEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.bans[entry.IP]; ok && existing.ExpiresAt.After(entry.ExpiresAt) {
		return existing
	}
	if len(s.bans) >= s.max {
		s.evictExpiredLocked(entry.BannedAt)
		if len(s.bans) >= s.max {
			// Still full of live bans: keep the existing ones rather than evicting
			// an arbitrary victim, and refuse the new one. The request is still
			// blocked by the limit that triggered this call; only the persistence
			// of the ban is dropped.
			return entry
		}
	}
	s.bans[entry.IP] = entry
	return entry
}

func (s *banStore) evictExpiredLocked(now time.Time) {
	for ip, e := range s.bans {
		if !now.Before(e.ExpiresAt) {
			delete(s.bans, ip)
		}
	}
}

// release removes a ban ahead of its expiry, reporting whether one was in force.
func (s *banStore) release(ip string) (BanEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.bans[ip]
	if ok {
		delete(s.bans, ip)
	}
	return entry, ok
}

// snapshot lists the bans still in force, newest first.
func (s *banStore) snapshot(now time.Time) []BanEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]BanEntry, 0, len(s.bans))
	for _, e := range s.bans {
		if now.Before(e.ExpiresAt) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BannedAt.After(out[j].BannedAt) })
	return out
}

// Enforcer holds the gateway's live enforcement state.
//
// It is created ONCE per process and is deliberately NOT rebuilt when the config
// reloads: who is currently banned, and how close a client is to a threshold,
// must survive an unrelated policy save. Container restart still clears it —
// that is a real, documented behaviour, not the upstream product's semantics.
type Enforcer struct {
	bans    *banStore
	limiter *rateLimiter
	journal *EventJournal
	now     func() time.Time
}

func NewEnforcer(journal *EventJournal) *Enforcer {
	return &Enforcer{
		bans:    newBanStore(maxTrackedBans),
		limiter: newRateLimiter(maxTrackedRateKeys),
		journal: journal,
		now:     time.Now,
	}
}

// Banned reports the ban in force for ip, if any.
func (e *Enforcer) Banned(ip string) (BanEntry, bool) {
	if e == nil {
		return BanEntry{}, false
	}
	return e.bans.active(ip, e.now())
}

// Release lifts a ban manually.
func (e *Enforcer) Release(ip string) (BanEntry, bool) {
	if e == nil {
		return BanEntry{}, false
	}
	entry, ok := e.bans.release(ip)
	if ok {
		e.journal.Record(EnforcementEvent{
			Kind:      EventBanReleased,
			WebsiteID: entry.WebsiteID,
			Host:      entry.Host,
			ClientIP:  entry.IP,
			Rule:      string(entry.Kind),
			Action:    "released",
		})
	}
	return entry, ok
}

// Bans lists the bans currently in force.
func (e *Enforcer) Bans() []BanEntry {
	if e == nil {
		return nil
	}
	return e.bans.snapshot(e.now())
}

// rateLimitOutcome is what a single counted event produced.
type rateLimitOutcome struct {
	// Triggered is true when the window total reached the configured threshold.
	Triggered bool
	// Banned is true when a ban was actually installed. A limit configured with
	// no ban duration triggers and is recorded, but lets the request through.
	Banned bool
	Kind   RateLimitKind
	Rule   string
}

// count records one event against a limit and applies its ban when the
// threshold is reached.
func (e *Enforcer) count(site siteRef, cfg RateLimitConfig, ip, target string) rateLimitOutcome {
	if e == nil || ip == "" {
		return rateLimitOutcome{}
	}
	now := e.now()
	key := rateKey(site.Host, cfg.Kind, ip, target)
	if !cfg.countsPerURL() {
		key = rateKey(site.Host, cfg.Kind, ip, "")
	}
	total := e.limiter.hit(key, cfg.PeriodSec, now.Unix())
	if total < cfg.Threshold {
		return rateLimitOutcome{}
	}
	out := rateLimitOutcome{Triggered: true, Kind: cfg.Kind, Rule: "ratelimit:" + string(cfg.Kind)}
	if cfg.BanSec <= 0 {
		return out
	}
	entry := e.bans.add(BanEntry{
		IP:        ip,
		Kind:      cfg.Kind,
		Host:      site.Host,
		WebsiteID: site.WebsiteID,
		BannedAt:  now,
		ExpiresAt: now.Add(time.Duration(cfg.BanSec) * time.Second),
	})
	out.Banned = true
	e.journal.Record(EnforcementEvent{
		Kind:      EventBan,
		WebsiteID: site.WebsiteID,
		Host:      site.Host,
		ClientIP:  ip,
		Rule:      out.Rule,
		Action:    "banned",
		Detail:    banDetail(entry),
	})
	return out
}

func banDetail(e BanEntry) string {
	return "expiresAt=" + e.ExpiresAt.UTC().Format(time.RFC3339)
}

// CountRequest applies every request-time limit configured for a site and
// reports the first one that fired.
func (e *Enforcer) CountRequest(site siteRef, limits []RateLimitConfig, r *http.Request) rateLimitOutcome {
	if e == nil || len(limits) == 0 {
		return rateLimitOutcome{}
	}
	ip := clientIPString(r.RemoteAddr)
	target := r.URL.Path
	var fired rateLimitOutcome
	for _, cfg := range limits {
		if cfg.Kind != RateLimitAccess && cfg.Kind != RateLimitURL {
			continue
		}
		// Every configured limit is counted, even after one has already fired:
		// skipping the rest would leave their windows permanently short.
		if out := e.count(site, cfg, ip, target); out.Triggered && !fired.Triggered {
			fired = out
		}
	}
	return fired
}

// CountStatus applies the response-status limits (currently 404 scanning).
func (e *Enforcer) CountStatus(site siteRef, limits []RateLimitConfig, r *http.Request, status int) rateLimitOutcome {
	if e == nil || len(limits) == 0 || status != http.StatusNotFound {
		return rateLimitOutcome{}
	}
	ip := clientIPString(r.RemoteAddr)
	var fired rateLimitOutcome
	for _, cfg := range limits {
		if cfg.Kind != RateLimitNotFound {
			continue
		}
		if out := e.count(site, cfg, ip, r.URL.Path); out.Triggered && !fired.Triggered {
			fired = out
		}
	}
	return fired
}
