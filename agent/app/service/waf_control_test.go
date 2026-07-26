package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
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

type fakeWafRuntime struct {
	checkErr       error
	reloadErr      error
	checkCalls     int
	reloadCalls    int
	failReloadOnce bool
}

func (*fakeWafRuntime) Up(string) error      { return nil }
func (*fakeWafRuntime) Restart(string) error { return nil }
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
