package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	wafasset "github.com/1Panel-dev/1Panel/agent/cmd/server/waf"
)

func TestSwitchRootProxyAndRoutingDetection(t *testing.T) {
	file := filepath.Join(t.TempDir(), "root.conf")
	original := `location ^~ / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}`
	if err := os.WriteFile(file, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := switchRootProxy(file, wafGatewayProxyPass); err != nil {
		t.Fatal(err)
	}
	if !isWafRouted(file) {
		t.Fatal("proxy should be recognized as routed through WAF")
	}
	content, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(content), "client_max_body_size 13m;") {
		t.Fatalf("WAF route must align nginx request body limit: %q err=%v", content, err)
	}
	if err := restoreRootProxy(file, "http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if isWafRouted(file) {
		t.Fatal("restored origin must not be reported as WAF-routed")
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != original {
		t.Fatalf("origin restore mismatch: %q err=%v", got, err)
	}
}

func TestNormalizedWebsiteOrigin(t *testing.T) {
	if got := normalizedWebsiteOrigin(model.Website{Proxy: "127.0.0.1:8080"}); got != "http://127.0.0.1:8080" {
		t.Fatalf("got %q", got)
	}
	if got := normalizedWebsiteOrigin(model.Website{Proxy: "https://origin.example/app"}); got != "https://origin.example/app" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitIPLines(t *testing.T) {
	if got := splitIPLines(""); got != nil {
		t.Fatalf("empty text must yield nil, got %#v", got)
	}
	if got := splitIPLines("   \n  \n"); got != nil {
		t.Fatalf("blank-only text must yield nil, got %#v", got)
	}
	got := splitIPLines("1.2.3.4\n 10.0.0.0/8 \n\n2001:db8::1")
	want := []string{"1.2.3.4", "10.0.0.0/8", "2001:db8::1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestNormalizedPolicyModeDefaultsToDetection(t *testing.T) {
	if got := normalizedPolicyMode(""); got != "detection" {
		t.Fatalf("got %q", got)
	}
	if got := normalizedPolicyMode("block"); got != "block" {
		t.Fatalf("got %q", got)
	}
}

func TestEffectivePolicyModeResolution(t *testing.T) {
	cases := []struct {
		site, global, want string
	}{
		{"block", "detection", "block"},         // explicit site mode wins
		{"detection", "block", "detection"},     // explicit site mode wins both ways
		{wafModeInherit, "block", "block"},      // inherit follows the global default
		{wafModeInherit, "", "detection"},       // blank global degrades to detection
		{"", "block", "block"},                  // legacy blank site mode behaves as inherit
		{" inherit ", "detection", "detection"}, // whitespace-tolerant
	}
	for _, c := range cases {
		if got := effectivePolicyMode(c.site, c.global); got != c.want {
			t.Fatalf("effectivePolicyMode(%q, %q) = %q, want %q", c.site, c.global, got, c.want)
		}
	}
}

func TestComposeAssetVersion(t *testing.T) {
	// A deployed file written before the marker existed must read as outdated.
	if got := composeAssetVersion([]byte("services:\n  waf-gateway:\n")); got != 0 {
		t.Fatalf("unmarked compose must be version 0, got %d", got)
	}
	if got := composeAssetVersion([]byte("# 1panel-x-waf-compose-version: 3\nservices:\n")); got != 3 {
		t.Fatalf("got %d", got)
	}
	if got := composeAssetVersion([]byte("# 1panel-x-waf-compose-version: nope\n")); got != 0 {
		t.Fatalf("unparsable marker must degrade to 0, got %d", got)
	}
	// The embedded asset must carry a marker, otherwise packaging changes could
	// never reach an installation that already has a compose file.
	if got := composeAssetVersion(wafasset.Compose); got < 1 {
		t.Fatalf("embedded compose asset must carry a version marker, got %d", got)
	}
}

type fakeWafRuntime struct {
	checkErr       error
	reloadErr      error
	checkCalls     int
	reloadCalls    int
	upCalls        int
	restartCalls   int
	failReloadOnce bool
}

func (f *fakeWafRuntime) Up(string) error      { f.upCalls++; return nil }
func (f *fakeWafRuntime) Restart(string) error { f.restartCalls++; return nil }
func (*fakeWafRuntime) NginxInstall() (model.AppInstall, error) {
	return model.AppInstall{ContainerName: "openresty"}, nil
}
func (f *fakeWafRuntime) NginxCheck(string) error {
	f.checkCalls++
	return f.checkErr
}
func (f *fakeWafRuntime) NginxReload(string) error {
	f.reloadCalls++
	if f.failReloadOnce && f.reloadCalls == 1 {
		return f.reloadErr
	}
	return nil
}

// newFakeGateway stands in for the WAF container's /healthz endpoint. The
// control plane addresses it at a fixed loopback port, so the test rewrites the
// service's HTTP client transport to reach the test server instead.
func newFakeGateway(t *testing.T, health func() gatewayHealth) *WafControlService {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(health())
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, target.Host)
			},
		},
	}
	return &WafControlService{
		healthClient:  client,
		runtime:       &fakeWafRuntime{},
		readyTimeout:  time.Second,
		reloadTimeout: time.Second,
	}
}

func TestApplyGatewayConfigPrefersInProcessReloadOverRestart(t *testing.T) {
	svc := newFakeGateway(t, func() gatewayHealth {
		return gatewayHealth{Status: "ready", Generation: "gen-2"}
	})
	rt := svc.runtime.(*fakeWafRuntime)

	if err := svc.applyGatewayConfig("gen-2", false); err != nil {
		t.Fatalf("healthy gateway on the requested generation must apply without a restart: %v", err)
	}
	// Restarting would erase the gateway's in-memory bans and rate-limit counters.
	if rt.upCalls != 0 || rt.restartCalls != 0 {
		t.Fatalf("no container churn expected: up=%d restart=%d", rt.upCalls, rt.restartCalls)
	}
}

func TestApplyGatewayConfigRestartsWhenComposeChanged(t *testing.T) {
	svc := newFakeGateway(t, func() gatewayHealth {
		return gatewayHealth{Status: "ready", Generation: "gen-2"}
	})
	rt := svc.runtime.(*fakeWafRuntime)

	// A rewritten compose file only takes effect once the container is recreated,
	// so the reload shortcut must be skipped even though the gateway looks ready.
	if err := svc.applyGatewayConfig("gen-2", true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rt.upCalls != 1 || rt.restartCalls != 1 {
		t.Fatalf("compose change must recreate the container: up=%d restart=%d", rt.upCalls, rt.restartCalls)
	}
}

func TestApplyGatewayConfigSurfacesGatewayRejection(t *testing.T) {
	svc := newFakeGateway(t, func() gatewayHealth {
		// Running an older generation and reporting why the new one was refused.
		return gatewayHealth{Status: "ready", Generation: "gen-1", LastError: "duplicate normalized host \"a.example\""}
	})

	err := svc.applyGatewayConfig("gen-2", false)
	if err == nil {
		t.Fatal("a refused configuration must not be reported as applied")
	}
	if !strings.Contains(err.Error(), "duplicate normalized host") {
		t.Fatalf("the gateway's reason must be surfaced, got %v", err)
	}
}

func TestApplyGatewayConfigFallsBackWhenGatewayIsDown(t *testing.T) {
	svc := newFakeGateway(t, func() gatewayHealth { return gatewayHealth{Status: "ready", Generation: "gen-2"} })
	rt := svc.runtime.(*fakeWafRuntime)
	// Point the client at a closed port so the health probe fails outright.
	svc.healthClient = &http.Client{Timeout: 100 * time.Millisecond, Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		},
	}}

	if err := svc.applyGatewayConfig("gen-2", false); err == nil {
		t.Fatal("an unreachable gateway must not be reported as applied")
	}
	if rt.upCalls != 1 || rt.restartCalls != 1 {
		t.Fatalf("an unreachable gateway must take the container path: up=%d restart=%d", rt.upCalls, rt.restartCalls)
	}
}

func TestApplyNginxRestoresConfigWhenCheckFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "root.conf")
	oldContent := []byte("old upstream")
	if err := os.WriteFile(file, []byte("new WAF upstream"), 0644); err != nil {
		t.Fatal(err)
	}
	runtime := &fakeWafRuntime{checkErr: errors.New("nginx -t failed")}
	service := &WafControlService{runtime: runtime}
	if err := service.applyNginx(file, oldContent, true, "openresty"); err == nil {
		t.Fatal("expected nginx check failure")
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != string(oldContent) {
		t.Fatalf("config was not restored: %q err=%v", got, err)
	}
	if runtime.checkCalls != 1 || runtime.reloadCalls != 0 {
		t.Fatalf("unexpected calls check=%d reload=%d", runtime.checkCalls, runtime.reloadCalls)
	}
}

func TestApplyNginxReloadsRestoredConfigAfterReloadFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "root.conf")
	oldContent := []byte("old upstream")
	if err := os.WriteFile(file, []byte("new WAF upstream"), 0644); err != nil {
		t.Fatal(err)
	}
	runtime := &fakeWafRuntime{reloadErr: errors.New("reload failed"), failReloadOnce: true}
	service := &WafControlService{runtime: runtime}
	if err := service.applyNginx(file, oldContent, true, "openresty"); err == nil {
		t.Fatal("expected nginx reload failure")
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != string(oldContent) {
		t.Fatalf("config was not restored: %q err=%v", got, err)
	}
	if runtime.checkCalls != 2 || runtime.reloadCalls != 2 {
		t.Fatalf("restored config must be checked and reloaded: check=%d reload=%d", runtime.checkCalls, runtime.reloadCalls)
	}
}
