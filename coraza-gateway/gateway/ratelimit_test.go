package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeClock drives the enforcer without sleeping.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestEnforcer(t *testing.T) (*Enforcer, *fakeClock, string) {
	t.Helper()
	journal, path := newJournalFixture(t)
	clock := &fakeClock{t: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)}
	e := NewEnforcer(journal)
	e.now = clock.now
	return e, clock, path
}

func TestSlidingCounterFiresExactlyAtThreshold(t *testing.T) {
	c := newSlidingCounter(10)
	base := int64(1000)
	for i := 1; i <= 200; i++ {
		if got := c.add(base); got != i {
			t.Fatalf("hit %d within the window should total %d, got %d", i, i, got)
		}
	}
	// Once the whole window has elapsed the count restarts from one.
	if got := c.add(base + 10); got != 1 {
		t.Fatalf("after the window elapsed the count must restart, got %d", got)
	}
}

func TestSlidingCounterDropsOnlyElapsedSeconds(t *testing.T) {
	c := newSlidingCounter(3)
	base := int64(1000)
	c.add(base)     // second 0
	c.add(base + 1) // second 1
	c.add(base + 2) // second 2 -> window holds 3
	if c.total != 3 {
		t.Fatalf("window total = %d, want 3", c.total)
	}
	// Advancing one second drops only the oldest bucket.
	if got := c.add(base + 3); got != 3 {
		t.Fatalf("expected 2 surviving + 1 new = 3, got %d", got)
	}
}

func TestSlidingCounterIgnoresBackwardsClock(t *testing.T) {
	c := newSlidingCounter(10)
	base := int64(1000)
	for i := 0; i < 5; i++ {
		c.add(base)
	}
	// A clock jumping backwards must not reset the window and hand the client a
	// fresh budget.
	if got := c.add(base - 100); got != 6 {
		t.Fatalf("backwards clock must not rewind the window, got %d", got)
	}
}

func TestAccessLimitBansAtThresholdAndExpires(t *testing.T) {
	e, clock, path := newTestEnforcer(t)
	site := siteRef{WebsiteID: 3, Host: "a.example"}
	cfg := RateLimitConfig{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 3, BanSec: 60}

	for i := 1; i < 3; i++ {
		if out := e.count(site, cfg, "203.0.113.9", "/"); out.Triggered {
			t.Fatalf("request %d must not trigger a threshold of 3", i)
		}
	}
	out := e.count(site, cfg, "203.0.113.9", "/")
	if !out.Triggered || !out.Banned {
		t.Fatalf("the 3rd request must trigger and ban, got %+v", out)
	}
	if _, ok := e.Banned("203.0.113.9"); !ok {
		t.Fatal("the client should now be banned")
	}
	// A different client is unaffected.
	if _, ok := e.Banned("203.0.113.10"); ok {
		t.Fatal("banning one client must not ban another")
	}

	clock.advance(61 * time.Second)
	if _, ok := e.Banned("203.0.113.9"); ok {
		t.Fatal("the ban must lapse once its duration has elapsed")
	}

	events := readJournal(t, path)
	if len(events) != 1 || events[0].Kind != EventBan || events[0].Action != "banned" {
		t.Fatalf("exactly one ban record expected, got %+v", events)
	}
	if !strings.Contains(events[0].Detail, "expiresAt=") {
		t.Fatalf("a ban record must carry its expiry so the panel can derive the state: %+v", events[0])
	}
}

func TestLimitWithoutBanDurationRecordsButLetsThrough(t *testing.T) {
	e, _, path := newTestEnforcer(t)
	site := siteRef{Host: "a.example"}
	// BanSec 0 is an explicitly detection-only limit.
	cfg := RateLimitConfig{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 2}

	e.count(site, cfg, "198.51.100.1", "/")
	out := e.count(site, cfg, "198.51.100.1", "/")
	if !out.Triggered {
		t.Fatal("the threshold should be reached")
	}
	if out.Banned {
		t.Fatal("a limit with no ban duration must not ban")
	}
	if _, ok := e.Banned("198.51.100.1"); ok {
		t.Fatal("no ban should exist")
	}
	if events := readJournal(t, path); len(events) != 0 {
		t.Fatalf("a detection-only trigger writes no ban record, got %+v", events)
	}
}

func TestPerURLLimitCountsTargetsSeparately(t *testing.T) {
	e, _, _ := newTestEnforcer(t)
	site := siteRef{Host: "a.example"}
	cfg := RateLimitConfig{Kind: RateLimitURL, PeriodSec: 10, Threshold: 2, BanSec: 60}

	e.count(site, cfg, "203.0.113.1", "/a")
	if out := e.count(site, cfg, "203.0.113.1", "/b"); out.Triggered {
		t.Fatal("a different target must have its own window")
	}
	if out := e.count(site, cfg, "203.0.113.1", "/a"); !out.Triggered {
		t.Fatal("the second hit on the same target must trigger")
	}
}

func TestGlobalModeCountsAllTargetsTogether(t *testing.T) {
	e, _, _ := newTestEnforcer(t)
	site := siteRef{Host: "a.example"}
	cfg := RateLimitConfig{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 2, BanSec: 60}

	e.count(site, cfg, "203.0.113.1", "/a")
	if out := e.count(site, cfg, "203.0.113.1", "/b"); !out.Triggered {
		t.Fatal("global mode must add different targets to the same window")
	}
}

func TestLimitsAreScopedPerSite(t *testing.T) {
	e, _, _ := newTestEnforcer(t)
	cfg := RateLimitConfig{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 2, BanSec: 60}

	e.count(siteRef{Host: "a.example"}, cfg, "203.0.113.1", "/")
	if out := e.count(siteRef{Host: "b.example"}, cfg, "203.0.113.1", "/"); out.Triggered {
		t.Fatal("one site's traffic must not consume another site's budget")
	}
}

func TestTrackedKeysStayBoundedUnderUniqueIPFlood(t *testing.T) {
	e, _, _ := newTestEnforcer(t)
	e.limiter = newRateLimiter(256)
	site := siteRef{Host: "a.example"}
	cfg := RateLimitConfig{Kind: RateLimitAccess, PeriodSec: 60, Threshold: 100, BanSec: 60}

	for i := 0; i < 5000; i++ {
		e.count(site, cfg, fmt.Sprintf("10.%d.%d.%d", i/65536, (i/256)%256, i%256), "/")
	}
	if size := e.limiter.size(); size > 256 {
		t.Fatalf("counter map must stay bounded, got %d entries", size)
	}
	// The overflow is reported rather than silently degrading enforcement.
	if !e.limiter.overflowed() {
		t.Fatal("dropping counters under pressure must be observable")
	}
}

func TestBanBlocksBeforeCountingAndSurvivesReload(t *testing.T) {
	var reached bool
	origin := httptest.NewServer(recordingUpstream(&reached))
	defer origin.Close()

	journal, path := newJournalFixture(t)
	clock := &fakeClock{t: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)}
	enforcer := NewEnforcer(journal)
	enforcer.now = clock.now

	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	limits := []RateLimitConfig{{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 3, BanSec: 60}}
	build := func(cfg Config) (*Router, error) {
		return NewRouterWithEnforcer(cfg, engine, ModeBlock, "X-Real-IP", journal, enforcer)
	}
	cfg := Config{Sites: []SiteConfig{{WebsiteID: 1, Host: "a.example", Upstream: origin.URL, RateLimits: limits}}}
	rt, err := build(cfg)
	if err != nil {
		t.Fatal(err)
	}

	hit := func(h http.Handler) int {
		req := httptest.NewRequest("GET", "http://a.example/", strings.NewReader(""))
		req.Header.Set("X-Real-IP", "203.0.113.77")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 1; i <= 2; i++ {
		if code := hit(rt); code != http.StatusOK {
			t.Fatalf("request %d should pass, got %d", i, code)
		}
	}
	if code := hit(rt); code != http.StatusForbidden {
		t.Fatalf("the request that reaches the threshold must be refused, got %d", code)
	}

	// Rebuild the routing table exactly as a config reload does. The enforcer is
	// owned by the process, so the ban must still be in force.
	reloaded, err := build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if code := hit(reloaded); code != http.StatusForbidden {
		t.Fatalf("a config reload must not un-ban anyone, got %d", code)
	}

	events := readJournal(t, path)
	var bans, blocked int
	for _, e := range events {
		switch e.Kind {
		case EventBan:
			bans++
		case EventBanned:
			blocked++
		}
	}
	if bans != 1 {
		t.Fatalf("expected exactly one ban record, got %d (%+v)", bans, events)
	}
	if blocked < 1 {
		t.Fatalf("requests refused by an existing ban must be recorded, got %d", blocked)
	}
}

func TestAllowListedClientIsExemptFromBans(t *testing.T) {
	var reached bool
	origin := httptest.NewServer(recordingUpstream(&reached))
	defer origin.Close()

	journal, _ := newJournalFixture(t)
	enforcer := NewEnforcer(journal)
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sites: []SiteConfig{{
		WebsiteID: 1,
		Host:      "a.example",
		Upstream:  origin.URL,
		AllowIPs:  []string{"203.0.113.88"},
		// A threshold of 1 would ban on the very first request if the allow list
		// did not take precedence.
		RateLimits: []RateLimitConfig{{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 1, BanSec: 60}},
	}}}
	rt, err := NewRouterWithEnforcer(cfg, engine, ModeBlock, "X-Real-IP", journal, enforcer)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "http://a.example/", strings.NewReader(""))
		req.Header.Set("X-Real-IP", "203.0.113.88")
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("an explicitly allow-listed client must never be rate-limited, got %d on request %d", rec.Code, i+1)
		}
	}
	if _, ok := enforcer.Banned("203.0.113.88"); ok {
		t.Fatal("an allow-listed client must never end up banned")
	}
}

func TestReleaseLiftsBanAndRecordsIt(t *testing.T) {
	e, _, path := newTestEnforcer(t)
	site := siteRef{WebsiteID: 2, Host: "a.example"}
	cfg := RateLimitConfig{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 1, BanSec: 600}
	e.count(site, cfg, "203.0.113.5", "/")

	if _, ok := e.Banned("203.0.113.5"); !ok {
		t.Fatal("client should be banned")
	}
	if _, ok := e.Release("203.0.113.5"); !ok {
		t.Fatal("release should report that a ban was lifted")
	}
	if _, ok := e.Banned("203.0.113.5"); ok {
		t.Fatal("the ban should be gone")
	}
	if _, ok := e.Release("203.0.113.5"); ok {
		t.Fatal("releasing an unbanned client must report that nothing was lifted")
	}

	var released int
	for _, ev := range readJournal(t, path) {
		if ev.Kind == EventBanReleased {
			released++
		}
	}
	if released != 1 {
		t.Fatalf("exactly one release record expected, got %d", released)
	}
}

func TestLongerBanWins(t *testing.T) {
	e, _, _ := newTestEnforcer(t)
	site := siteRef{Host: "a.example"}
	long := RateLimitConfig{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 1, BanSec: 3600}
	short := RateLimitConfig{Kind: RateLimitURL, PeriodSec: 10, Threshold: 1, BanSec: 5}

	e.count(site, long, "203.0.113.6", "/")
	before, _ := e.Banned("203.0.113.6")
	e.count(site, short, "203.0.113.6", "/")
	after, ok := e.Banned("203.0.113.6")
	if !ok {
		t.Fatal("client should still be banned")
	}
	if !after.ExpiresAt.Equal(before.ExpiresAt) {
		t.Fatal("a shorter ban must not cut an existing longer one short")
	}
}

func TestNotFoundLimitCountsOnlyMissingResponses(t *testing.T) {
	e, _, _ := newTestEnforcer(t)
	site := siteRef{Host: "a.example"}
	limits := []RateLimitConfig{{Kind: RateLimitNotFound, PeriodSec: 10, Threshold: 2, BanSec: 60}}
	req := httptest.NewRequest("GET", "http://a.example/missing", strings.NewReader(""))
	req.RemoteAddr = "203.0.113.20:1111"

	if out := e.CountStatus(site, limits, req, http.StatusOK); out.Triggered {
		t.Fatal("a 200 must not feed the 404 limit")
	}
	e.CountStatus(site, limits, req, http.StatusNotFound)
	if out := e.CountStatus(site, limits, req, http.StatusNotFound); !out.Triggered {
		t.Fatal("the second 404 must reach the threshold")
	}
}

func TestParseConfigValidatesRateLimits(t *testing.T) {
	bad := []string{
		`{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rateLimits":[{"kind":"nope","periodSec":10,"threshold":5}]}]}`,
		`{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rateLimits":[{"kind":"access","periodSec":0,"threshold":5}]}]}`,
		`{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rateLimits":[{"kind":"access","periodSec":99999,"threshold":5}]}]}`,
		`{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rateLimits":[{"kind":"access","periodSec":10,"threshold":0}]}]}`,
		`{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rateLimits":[{"kind":"access","periodSec":10,"threshold":5,"banSec":-1}]}]}`,
		`{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rateLimits":[{"kind":"access","periodSec":10,"threshold":5},{"kind":"access","periodSec":20,"threshold":9}]}]}`,
	}
	for i, body := range bad {
		if _, err := ParseConfig([]byte(body)); err == nil {
			t.Fatalf("case %d must be rejected at load time: %s", i, body)
		}
	}
	good := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rateLimits":[{"kind":"access","periodSec":10,"threshold":200,"banSec":600,"perUrl":true},{"kind":"notfound","periodSec":60,"threshold":30,"banSec":600}]}]}`
	if _, err := ParseConfig([]byte(good)); err != nil {
		t.Fatalf("valid limits rejected: %v", err)
	}
}

func TestNilEnforcerIsSafe(t *testing.T) {
	var e *Enforcer
	if _, ok := e.Banned("1.2.3.4"); ok {
		t.Fatal("a nil enforcer must report no bans")
	}
	if _, ok := e.Release("1.2.3.4"); ok {
		t.Fatal("a nil enforcer must release nothing")
	}
	if e.Bans() != nil {
		t.Fatal("a nil enforcer must have no bans")
	}
	req := httptest.NewRequest("GET", "/", strings.NewReader(""))
	if out := e.CountRequest(siteRef{}, nil, req); out.Triggered {
		t.Fatal("a nil enforcer must not trigger")
	}
	if out := e.CountStatus(siteRef{}, nil, req, 404); out.Triggered {
		t.Fatal("a nil enforcer must not trigger")
	}
}
