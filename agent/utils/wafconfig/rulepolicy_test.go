package wafconfig

import (
	"reflect"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
)

func TestNormalizeMethodsCanonicalizes(t *testing.T) {
	got, err := NormalizeMethods([]string{" post ", "GET", "get", "", "Delete"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"DELETE", "GET", "POST"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	if empty, err := NormalizeMethods(nil); err != nil || empty != nil {
		t.Fatalf("an empty list must stay empty: %#v %v", empty, err)
	}
}

// The method list ends up inside an engine directive, so a token carrying a
// quote or a newline could append arbitrary directives — and "SecRuleEngine
// Off" would silently disable the WAF for every site sharing that engine.
func TestNormalizeMethodsRejectsDirectiveInjection(t *testing.T) {
	bad := []string{
		"GET'\nSecRuleEngine Off\n",
		"GET\nSecRuleEngine Off",
		"GET POST",
		"GET;",
		"'",
		"\"",
		strings.Repeat("A", 21),
	}
	for _, m := range bad {
		if _, err := NormalizeMethods([]string{m}); err == nil {
			t.Fatalf("method %q must be rejected", m)
		}
	}
	if _, err := NormalizeMethods(make([]string, MaxAllowedMethods+1)); err == nil {
		t.Fatal("an oversized method list must be rejected")
	}
}

func TestRulePolicyZeroIsFullProtection(t *testing.T) {
	if !(RulePolicy{}).IsZero() {
		t.Fatal("the zero policy must be recognised as the default")
	}
	for name, p := range map[string]RulePolicy{
		"sqli off": {DisableSQLi: true},
		"xss off":  {DisableXSS: true},
		"strict":   {Strict: true},
		"methods":  {AllowedMethods: []string{"GET"}},
		"uploads":  {BannedUploadExts: []string{"php"}},
	} {
		if p.IsZero() {
			t.Fatalf("%s must not be treated as the default policy", name)
		}
	}
}

// A boolean cannot express "not set", so merging field by field would make it
// impossible for a site to switch something back ON once the panel default
// switched it off. The whole policy is therefore taken from one side or the
// other.
func TestMergeRulePolicyTakesTheSitePolicyWhole(t *testing.T) {
	global := &RulePolicy{DisableSQLi: true, Strict: true}
	site := &RulePolicy{DisableXSS: true}

	got := MergeRulePolicy(global, site)
	if got != site {
		t.Fatalf("a site policy must win whole, got %+v", got)
	}
	if got.DisableSQLi || got.Strict {
		t.Fatalf("the panel default must not bleed into a site policy: %+v", got)
	}
	if MergeRulePolicy(global, nil) != global {
		t.Fatal("a site with no policy must inherit the panel default")
	}
	if MergeRulePolicy(nil, nil) != nil {
		t.Fatal("no policy anywhere must stay absent")
	}
}

func TestBuildEmitsRulePolicyAndOmitsTheDefault(t *testing.T) {
	mk := func(rules *RulePolicy) GatewayConfig {
		cfg, err := Build([]Site{{
			Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
			Domains: []model.WebsiteDomain{{Domain: "a.example"}},
			Enabled: true,
			Rules:   rules,
		}})
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	// An all-default policy is emitted as ABSENT, so an older gateway that does
	// not know the field still enforces full protection.
	if got := mk(&RulePolicy{}); got.Sites[0].Rules != nil {
		t.Fatalf("the default policy must be omitted, got %+v", got.Sites[0].Rules)
	}
	strict := mk(&RulePolicy{Strict: true, AllowedMethods: []string{"post", "GET"}})
	if strict.Sites[0].Rules == nil || !strict.Sites[0].Rules.Strict {
		t.Fatalf("a non-default policy must reach the emitted site: %+v", strict.Sites[0].Rules)
	}
	if !reflect.DeepEqual(strict.Sites[0].Rules.AllowedMethods, []string{"GET", "POST"}) {
		t.Fatalf("methods must be canonicalized: %#v", strict.Sites[0].Rules.AllowedMethods)
	}
	if mk(nil).Generation == strict.Generation {
		t.Fatal("a rule policy must change the config generation")
	}
	// Equivalent policies must not churn the generation, so an unchanged policy
	// never forces a needless gateway reload.
	a := mk(&RulePolicy{AllowedMethods: []string{"GET", "POST"}})
	b := mk(&RulePolicy{AllowedMethods: []string{"post", "get"}})
	if a.Generation != b.Generation {
		t.Fatalf("equivalent policies must produce a stable generation: %s vs %s", a.Generation, b.Generation)
	}
}

func TestBuildRejectsInvalidRulePolicy(t *testing.T) {
	mk := func(rules *RulePolicy) error {
		_, err := Build([]Site{{
			Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
			Domains: []model.WebsiteDomain{{Domain: "a.example"}},
			Enabled: true,
			Rules:   rules,
		}})
		return err
	}
	if mk(&RulePolicy{AllowedMethods: []string{"GET\nSecRuleEngine Off"}}) == nil {
		t.Fatal("a method token that could inject directives must fail config generation")
	}
	// The extension list is interpolated into a SecRule regular expression, so
	// the same class of injection has to be refused here too.
	if mk(&RulePolicy{BannedUploadExts: []string{`php" "id:1,phase:1,pass"`}}) == nil {
		t.Fatal("an upload extension that could inject directives must fail config generation")
	}
}

func TestNormalizeUploadExtensions(t *testing.T) {
	got, err := NormalizeUploadExtensions([]string{" .PHP ", "jsp", "php", "", "."})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"jsp", "php"}) {
		t.Fatalf("extensions must be dot-stripped, lower-cased, de-duplicated and sorted: %#v", got)
	}
	for _, bad := range []string{`php"`, "php\nSecRuleEngine Off", "php|jsp", "p.p", "php)("} {
		if _, err := NormalizeUploadExtensions([]string{bad}); err == nil {
			t.Fatalf("extension %q must be rejected", bad)
		}
	}
	if _, err := NormalizeUploadExtensions(make([]string, MaxBannedUploadExtensions+1)); err == nil {
		t.Fatal("an oversized extension list must be rejected")
	}
}
