package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// buildObservedGateway wires an engine to a real enforcer so rule matches feed
// the attack-frequency limit, in whichever mode the case needs.
func buildObservedGateway(t *testing.T, mode Mode, limit *RateLimitConfig, reached *bool) (*Handler, *Enforcer, string) {
	t.Helper()
	journal, path := newJournalFixture(t)
	enforcer := NewEnforcer(journal)
	enforcer.SetAttackLimit(limit)
	engine, err := NewEngineWithObserver(mode, 1<<20, "", enforcer.AttackObserver())
	if err != nil {
		t.Fatalf("engine build failed: %v", err)
	}
	h := NewHandler(engine, recordingUpstream(reached), mode).
		WithRealIPHeader("X-Real-IP").
		WithSite(siteRef{WebsiteID: 1, Host: "a.example"}).
		WithJournal(journal).
		WithEnforcer(enforcer, nil)
	return h, enforcer, path
}

func attackFrom(h http.Handler, ip, target string) int {
	req := httptest.NewRequest("GET", target, strings.NewReader(""))
	req.Header.Set("X-Real-IP", ip)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// This is the load-bearing assertion for the attack limit: detection mode never
// interrupts, so without the engine's match callback an attack would be
// completely invisible and the limit could never fire.
func TestAttackLimitCountsInDetectionModeToo(t *testing.T) {
	for _, mode := range []Mode{ModeBlock, ModeDetection} {
		t.Run(string(mode), func(t *testing.T) {
			var reached bool
			limit := &RateLimitConfig{Kind: RateLimitAttack, PeriodSec: 60, Threshold: 3, BanSec: 300}
			h, enforcer, _ := buildObservedGateway(t, mode, limit, &reached)

			for i := 1; i <= 2; i++ {
				attackFrom(h, "203.0.113.30", sqliTarget)
				if _, banned := enforcer.Banned("203.0.113.30"); banned {
					t.Fatalf("banned after only %d attacks, threshold is 3", i)
				}
			}
			attackFrom(h, "203.0.113.30", sqliTarget)
			if _, banned := enforcer.Banned("203.0.113.30"); !banned {
				t.Fatalf("mode %s: three attacks must reach the threshold", mode)
			}
		})
	}
}

func TestAttackLimitCountsOneAttackPerRequest(t *testing.T) {
	var reached bool
	// A threshold of 2 only holds if the many CRS rules one hostile request
	// matches are collapsed into a single counted attack.
	limit := &RateLimitConfig{Kind: RateLimitAttack, PeriodSec: 60, Threshold: 2, BanSec: 300}
	h, enforcer, _ := buildObservedGateway(t, ModeBlock, limit, &reached)

	attackFrom(h, "203.0.113.31", sqliTarget)
	if _, banned := enforcer.Banned("203.0.113.31"); banned {
		t.Fatal("one hostile request must count as exactly one attack, not one per matched rule")
	}
	attackFrom(h, "203.0.113.31", xssTarget)
	if _, banned := enforcer.Banned("203.0.113.31"); !banned {
		t.Fatal("the second hostile request must reach the threshold")
	}
}

func TestCleanTrafficNeverFeedsTheAttackLimit(t *testing.T) {
	var reached bool
	limit := &RateLimitConfig{Kind: RateLimitAttack, PeriodSec: 60, Threshold: 2, BanSec: 300}
	h, enforcer, _ := buildObservedGateway(t, ModeBlock, limit, &reached)

	for i := 0; i < 20; i++ {
		if code := attackFrom(h, "203.0.113.32", "/products?page=2&sort=name"); code != http.StatusOK {
			t.Fatalf("clean request %d should pass, got %d", i, code)
		}
	}
	if _, banned := enforcer.Banned("203.0.113.32"); banned {
		t.Fatal("ordinary traffic must never trip the attack limit")
	}
}

func TestAttackLimitDisabledWhenUnconfigured(t *testing.T) {
	var reached bool
	h, enforcer, _ := buildObservedGateway(t, ModeBlock, nil, &reached)
	for i := 0; i < 10; i++ {
		attackFrom(h, "203.0.113.33", sqliTarget)
	}
	if _, banned := enforcer.Banned("203.0.113.33"); banned {
		t.Fatal("no attack limit configured means no ban")
	}
}

func TestAttackDeduperBoundsMemoryAndCountsEachTransactionOnce(t *testing.T) {
	d := newAttackDeduper(4)
	if !d.first("tx-1") {
		t.Fatal("first sight of a transaction must count")
	}
	if d.first("tx-1") {
		t.Fatal("a repeat match on the same transaction must not count again")
	}
	for i := 0; i < 100; i++ {
		d.first(string(rune('a'+i%26)) + "-" + time.Duration(i).String())
	}
	if len(d.seen) > 4 || len(d.fifo) > 4 {
		t.Fatalf("dedupe set must stay bounded, got seen=%d fifo=%d", len(d.seen), len(d.fifo))
	}
	// An empty id cannot be correlated, so it is counted rather than dropped.
	if !d.first("") {
		t.Fatal("an unidentifiable match must be counted, not silently discarded")
	}
}

func TestParseConfigRejectsPerSiteAttackLimit(t *testing.T) {
	body := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rateLimits":[{"kind":"attack","periodSec":60,"threshold":5,"banSec":600}]}]}`
	if _, err := ParseConfig([]byte(body)); err == nil {
		t.Fatal("a per-site attack limit must be refused: the data plane cannot honour it per site")
	}
	ok := `{"attackRateLimit":{"periodSec":60,"threshold":5,"banSec":600},"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1"}]}`
	cfg, err := ParseConfig([]byte(ok))
	if err != nil {
		t.Fatalf("gateway-wide attack limit rejected: %v", err)
	}
	if cfg.AttackRateLimit == nil || cfg.AttackRateLimit.Kind != RateLimitAttack {
		t.Fatalf("the kind should default to attack, got %+v", cfg.AttackRateLimit)
	}
}
