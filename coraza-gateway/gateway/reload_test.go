package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfigFile renders a config JSON for the given sites and writes it to path.
func writeConfigFile(t *testing.T, path, generation string, sites []SiteConfig) {
	t.Helper()
	data, err := json.Marshal(Config{Version: ConfigVersion, Generation: generation, Sites: sites})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// newReloadFixture builds a ReloadableRouter over a temp config file backed by a
// single shared engine, so a case costs one CRS compile rather than one per reload.
func newReloadFixture(t *testing.T, mode Mode, sites []SiteConfig) (*ReloadableRouter, string) {
	t.Helper()
	engine, err := NewEngine(mode, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "gateway.json")
	writeConfigFile(t, path, "gen-1", sites)
	build := func(cfg Config) (*Router, error) { return NewRouter(cfg, engine, mode, "X-Real-IP") }
	rr, err := NewReloadableRouter(path, mode, build)
	if err != nil {
		t.Fatal(err)
	}
	rr.logf = func(string, ...any) {}
	return rr, path
}

func TestReloadSwapsRoutingTableInProcess(t *testing.T) {
	var reached bool
	origin := httptest.NewServer(recordingUpstream(&reached))
	defer origin.Close()

	rr, path := newReloadFixture(t, ModeBlock, []SiteConfig{{Host: "a.example", Upstream: origin.URL}})

	if got := rr.HealthSnapshot(); got.Generation != "gen-1" || got.Sites != 1 {
		t.Fatalf("initial health = %+v", got)
	}
	if code := serve(rr, "GET", "http://b.example/", "").Code; code != http.StatusForbidden {
		t.Fatalf("b.example must be unknown before reload, got %d", code)
	}

	writeConfigFile(t, path, "gen-2", []SiteConfig{
		{Host: "a.example", Upstream: origin.URL},
		{Host: "b.example", Upstream: origin.URL},
	})
	if err := rr.ReloadFromFile(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if got := rr.HealthSnapshot(); got.Generation != "gen-2" || got.Sites != 2 || got.LastError != "" {
		t.Fatalf("health after reload = %+v", got)
	}
	if code := serve(rr, "GET", "http://b.example/", "").Code; code != http.StatusOK {
		t.Fatalf("b.example must be routed after reload, got %d", code)
	}
	if !reached {
		t.Fatal("reloaded site must reach its upstream")
	}
}

func TestReloadKeepsRunningPolicyWhenCandidateIsInvalid(t *testing.T) {
	var reached bool
	origin := httptest.NewServer(recordingUpstream(&reached))
	defer origin.Close()

	rr, path := newReloadFixture(t, ModeBlock, []SiteConfig{{Host: "a.example", Upstream: origin.URL}})

	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rr.ReloadFromFile(); err == nil {
		t.Fatal("malformed config must be rejected")
	}

	got := rr.HealthSnapshot()
	if got.LastError == "" {
		t.Fatal("rejection must be reported on /healthz")
	}
	// Generation keeps describing what is RUNNING, not what is on disk.
	if got.Generation != "gen-1" || got.Sites != 1 {
		t.Fatalf("running config must be preserved, got %+v", got)
	}
	// The previous policy must still be ENFORCED, not merely remembered.
	if code := serve(rr, "GET", "http://a.example"+sqliTarget, "").Code; code != http.StatusForbidden {
		t.Fatalf("attack must still be blocked after a rejected reload, got %d", code)
	}
	reached = false
	if code := serve(rr, "GET", "http://a.example/", "").Code; code != http.StatusOK || !reached {
		t.Fatalf("clean traffic must still be proxied after a rejected reload, got %d reached=%v", code, reached)
	}
}

func TestReloadSkipsUnchangedAndAlreadyRejectedContent(t *testing.T) {
	var reached bool
	origin := httptest.NewServer(recordingUpstream(&reached))
	defer origin.Close()

	rr, path := newReloadFixture(t, ModeBlock, []SiteConfig{{Host: "a.example", Upstream: origin.URL}})

	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	builds := 0
	rr.build = func(cfg Config) (*Router, error) {
		builds++
		return NewRouter(cfg, engine, ModeBlock, "X-Real-IP")
	}

	for i := 0; i < 3; i++ {
		if err := rr.ReloadFromFile(); err != nil {
			t.Fatalf("unchanged reload %d: %v", i, err)
		}
	}
	if builds != 0 {
		t.Fatalf("unchanged config must not rebuild, got %d builds", builds)
	}

	// A bad candidate is attempted exactly once, not retried on every tick.
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rr.ReloadFromFile(); err == nil {
		t.Fatal("expected rejection")
	}
	for i := 0; i < 3; i++ {
		if err := rr.ReloadFromFile(); err != nil {
			t.Fatalf("already-rejected content must be skipped, got %v", err)
		}
	}
}

func TestParseConfigRejectsNewerContractVersion(t *testing.T) {
	newer := fmt.Sprintf(`{"version":%d,"sites":[{"host":"a.example","upstream":"http://127.0.0.1:8080"}]}`, ConfigVersion+1)
	err := func() error { _, e := ParseConfig([]byte(newer)); return e }()
	if err == nil {
		t.Fatal("a newer contract version must be refused, not parsed leniently")
	}
	if !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, v := range []int{0, 1, ConfigVersion} {
		body := fmt.Sprintf(`{"version":%d,"sites":[{"host":"a.example","upstream":"http://127.0.0.1:8080"}]}`, v)
		if _, err := ParseConfig([]byte(body)); err != nil {
			t.Fatalf("version %d must be accepted: %v", v, err)
		}
	}
}

func TestRouterSharesOneEnginePerDistinctPolicy(t *testing.T) {
	engine, err := NewEngine(ModeDetection, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	// Six sites, two distinct policies: inherited detection + explicit block.
	sites := make([]SiteConfig, 0, 6)
	for i := 0; i < 6; i++ {
		s := SiteConfig{Host: fmt.Sprintf("s%d.example", i), Upstream: "http://127.0.0.1:8080"}
		if i%2 == 0 {
			s.Mode = ModeBlock
		}
		sites = append(sites, s)
	}
	rt, err := NewRouter(Config{Sites: sites}, engine, ModeDetection, "")
	if err != nil {
		t.Fatal(err)
	}
	if rt.engines != 2 {
		t.Fatalf("6 sites over 2 policies must compile 2 engines, got %d", rt.engines)
	}
}

func TestEngineCacheRefusesToCompilePastTheCap(t *testing.T) {
	base := &Engine{mode: ModeDetection, bodyLimit: 1 << 20}
	cache := newEngineCache(base, enginePolicy{Mode: ModeDetection})
	// Fill the cache without compiling: the cap must be enforced BEFORE any
	// expensive CRS build is attempted.
	for i := 1; i < maxCompiledEngines; i++ {
		p := enginePolicy{Mode: ModeDetection, BodyLimit: i}
		cache.compiled[p] = base
		cache.order = append(cache.order, p)
	}
	if cache.size() != maxCompiledEngines {
		t.Fatalf("fixture size = %d", cache.size())
	}
	if _, err := cache.get(enginePolicy{Mode: ModeBlock, BodyLimit: 999}, "overflow.example"); err == nil {
		t.Fatal("exceeding the compiled-policy cap must be an error, not a silent compile")
	} else if !strings.Contains(err.Error(), "overflow.example") {
		t.Fatalf("error must name the offending site, got %v", err)
	}
}
