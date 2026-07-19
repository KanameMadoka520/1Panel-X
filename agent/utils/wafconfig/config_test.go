package wafconfig

import (
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
)

func TestBuildDeterministicMultiDomainConfig(t *testing.T) {
	input := []Site{
		{
			Website: model.Website{BaseModel: model.BaseModel{ID: 2}, Type: constant.Proxy, Alias: "beta", Proxy: "127.0.0.1:8082"},
			Domains: []model.WebsiteDomain{{Domain: "B.example:443"}}, Enabled: true, Mode: ModeBlock,
		},
		{
			Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "https://origin.example/app"},
			Domains: []model.WebsiteDomain{{Domain: "z.example"}, {Domain: "a.example:80"}}, Enabled: true,
		},
		{Website: model.Website{BaseModel: model.BaseModel{ID: 3}, Type: constant.Proxy, Alias: "off", Proxy: "127.0.0.1:8083"}, Enabled: false},
	}
	cfg, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sites) != 3 || cfg.Version != Version {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.Sites[0].Host != "a.example" || cfg.Sites[0].Mode != ModeDetection {
		t.Fatalf("sites must be sorted and default to detection: %#v", cfg.Sites)
	}
	if cfg.Sites[0].Upstream != "https://origin.example/app" {
		t.Fatalf("origin path must be preserved: %q", cfg.Sites[0].Upstream)
	}
	if cfg.Sites[2].Host != "z.example" {
		t.Fatalf("unexpected sort order: %#v", cfg.Sites)
	}
}

func TestBuildRejectsUnsupportedAndConflictingSites(t *testing.T) {
	base := func(id uint, alias, typ, proxy string, domains ...string) Site {
		items := make([]model.WebsiteDomain, 0, len(domains))
		for _, domain := range domains {
			items = append(items, model.WebsiteDomain{Domain: domain})
		}
		return Site{Website: model.Website{BaseModel: model.BaseModel{ID: id}, Alias: alias, Type: typ, Proxy: proxy}, Domains: items, Enabled: true}
	}
	for name, input := range map[string][]Site{
		"unsupported type": {base(1, "static", constant.Static, "127.0.0.1:8080", "static.example")},
		"duplicate host":   {base(1, "a", constant.Proxy, "127.0.0.1:8080", "Example.com"), base(2, "b", constant.Proxy, "127.0.0.1:8081", "example.com:443")},
		"no domains":       {{Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Alias: "empty", Type: constant.Proxy, Proxy: "127.0.0.1:8080"}, Enabled: true}},
		"bad origin":       {base(1, "a", constant.Proxy, "file:///etc/passwd", "a.example")},
		"bad mode":         {{Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Alias: "a", Type: constant.Proxy, Proxy: "127.0.0.1:8080"}, Domains: []model.WebsiteDomain{{Domain: "a.example"}}, Enabled: true, Mode: "permit-all"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		" Example.COM:443 ": "example.com",
		"example.com.":      "example.com",
		"[2001:db8::1]:443": "2001:db8::1",
		"[2001:db8::1]":     "2001:db8::1",
		"2001:db8::1":       "2001:db8::1",
	}
	for input, want := range cases {
		got, err := normalizeHost(input)
		if err != nil || got != want {
			t.Errorf("normalizeHost(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "a/b", "a\\b", "a?b", "a#b", "a@b"} {
		if _, err := normalizeHost(input); err == nil {
			t.Errorf("normalizeHost(%q) should fail", input)
		}
	}
}

func TestMarshalStableShape(t *testing.T) {
	cfg, err := Build([]Site{{Website: model.Website{BaseModel: model.BaseModel{ID: 7}, Alias: "site", Type: constant.Proxy, Proxy: "127.0.0.1:8080"}, Domains: []model.WebsiteDomain{{Domain: "site.example"}}, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"version": 1`) || !strings.Contains(string(data), `"websiteId": 7`) {
		t.Fatalf("unexpected JSON: %s", data)
	}
}
