package gateway

import (
	"net/http"
	"strings"
	"testing"
)

// buildPolicyGateway compiles one engine under the given rule policy.
func buildPolicyGateway(t *testing.T, policy RulePolicy, reached *bool) *Handler {
	t.Helper()
	site := SiteConfig{Host: "a.example", Upstream: "http://127.0.0.1:1", Mode: ModeBlock, Rules: &policy}
	p, err := site.enginePolicy(ModeBlock)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	engine, err := newEngine(p, "", nil)
	if err != nil {
		t.Fatalf("engine build failed: %v", err)
	}
	return NewHandler(engine, recordingUpstream(reached), ModeBlock)
}

// This is the load-bearing assertion for the detection toggles: switching one
// class off must stop THAT class being blocked and leave every other class
// blocking. A toggle that reads "off" while the engine still blocks — or that
// takes the whole ruleset down with it — is the failure mode being guarded.
func TestSQLiToggleOnlyDisablesSQLi(t *testing.T) {
	var reached bool
	h := buildPolicyGateway(t, RulePolicy{DisableSQLi: true}, &reached)

	if code := serve(h, "GET", sqliTarget, "").Code; code != http.StatusOK {
		t.Fatalf("SQLi detection is off, the request must pass, got %d", code)
	}
	if !reached {
		t.Fatal("with SQLi off the request must actually reach the origin")
	}
	if code := serve(h, "GET", xssTarget, "").Code; code != http.StatusForbidden {
		t.Fatalf("XSS must still be blocked, got %d", code)
	}
	if code := serve(h, "GET", traversalTarget, "").Code; code != http.StatusForbidden {
		t.Fatalf("path traversal must still be blocked, got %d", code)
	}
}

func TestXSSToggleOnlyDisablesXSS(t *testing.T) {
	var reached bool
	h := buildPolicyGateway(t, RulePolicy{DisableXSS: true}, &reached)

	if code := serve(h, "GET", xssTarget, "").Code; code != http.StatusOK {
		t.Fatalf("XSS detection is off, the request must pass, got %d", code)
	}
	if code := serve(h, "GET", sqliTarget, "").Code; code != http.StatusForbidden {
		t.Fatalf("SQLi must still be blocked, got %d", code)
	}
}

func TestDefaultPolicyBlocksEverything(t *testing.T) {
	var reached bool
	// The zero policy must be the fully-protecting one.
	h := buildPolicyGateway(t, RulePolicy{}, &reached)
	for name, target := range map[string]string{"sqli": sqliTarget, "xss": xssTarget, "traversal": traversalTarget} {
		if code := serve(h, "GET", target, "").Code; code != http.StatusForbidden {
			t.Fatalf("%s must be blocked under the default policy, got %d", name, code)
		}
	}
	if code := serve(h, "GET", "/products?page=2", "").Code; code != http.StatusOK {
		t.Fatalf("clean traffic must pass, got %d", code)
	}
}

func TestMethodAllowListRefusesOtherMethods(t *testing.T) {
	var reached bool
	h := buildPolicyGateway(t, RulePolicy{AllowedMethods: []string{"get", "POST"}}, &reached)

	for _, m := range []string{"GET", "POST"} {
		reached = false
		if code := serve(h, m, "/", "").Code; code != http.StatusOK || !reached {
			t.Fatalf("%s is allow-listed and must pass, got %d reached=%v", m, code, reached)
		}
	}
	for _, m := range []string{"DELETE", "PUT", "TRACE"} {
		reached = false
		if code := serve(h, m, "/", "").Code; code != http.StatusForbidden {
			t.Fatalf("%s is not allow-listed and must be refused, got %d", m, code)
		}
		if reached {
			t.Fatalf("%s must not reach the origin", m)
		}
	}
}

func TestStrictModeRaisesParanoiaWithoutBreakingBaseline(t *testing.T) {
	var reached bool
	strict := buildPolicyGateway(t, RulePolicy{Strict: true}, &reached)

	// Strict mode must not break ordinary traffic...
	reached = false
	if code := serve(strict, "GET", "/products?page=2&sort=name", "").Code; code != http.StatusOK || !reached {
		t.Fatalf("clean traffic must still pass under strict mode, got %d reached=%v", code, reached)
	}
	// ...and must still block the baseline attacks.
	if code := serve(strict, "GET", sqliTarget, "").Code; code != http.StatusForbidden {
		t.Fatalf("SQLi must still be blocked under strict mode, got %d", code)
	}
}

func TestDirectiveOrderPlacesOverridesBeforeTheRuleInclude(t *testing.T) {
	got := directivesFor(enginePolicy{Mode: ModeBlock, Strict: true, AllowedMethods: "GET POST", DisableSQLi: true}, "")

	setupIdx := strings.Index(got, "Include @crs-setup.conf.example")
	rulesIdx := strings.Index(got, "Include @owasp_crs/*.conf")
	paranoiaIdx := strings.Index(got, "tx.blocking_paranoia_level")
	methodsIdx := strings.Index(got, "tx.allowed_methods")
	removeIdx := strings.Index(got, "SecRuleRemoveById")

	if setupIdx < 0 || rulesIdx < 0 || paranoiaIdx < 0 || methodsIdx < 0 || removeIdx < 0 {
		t.Fatalf("missing directive in:\n%s", got)
	}
	// The ruleset installs its defaults guarded by "variable was never set", and
	// phase-1 rules run in definition order, so an override is only honoured if
	// it is emitted before the rule include.
	if !(setupIdx < paranoiaIdx && paranoiaIdx < rulesIdx) {
		t.Fatalf("paranoia override must sit between the setup and rule includes:\n%s", got)
	}
	if !(setupIdx < methodsIdx && methodsIdx < rulesIdx) {
		t.Fatalf("method override must sit between the setup and rule includes:\n%s", got)
	}
	// Removals only work once the rules exist.
	if removeIdx < rulesIdx {
		t.Fatalf("removals must come after the rule include:\n%s", got)
	}
	// SecRuleUpdateActionById hard-errors on a missing id, so one ruleset bump
	// would brick every site that used it. It must never be generated.
	if strings.Contains(got, "SecRuleUpdateActionById") {
		t.Fatalf("SecRuleUpdateActionById must never be emitted:\n%s", got)
	}
	// Structurally load-bearing families must never be removable.
	for _, protected := range []string{"901", "905", "949", "959", "980"} {
		if strings.Contains(got, "SecRuleRemoveById "+protected) {
			t.Fatalf("rule family %s is load-bearing and must never be removed:\n%s", protected, got)
		}
	}
}

func TestMethodNormalizationRejectsDirectiveInjection(t *testing.T) {
	// The list is interpolated into a SecAction; a value carrying a quote or a
	// newline could append arbitrary directives, and "SecRuleEngine Off" would
	// disable the WAF for every site sharing the compiled engine.
	bad := []string{
		"GET'\nSecRuleEngine Off\n",
		"GET\nSecRuleEngine Off",
		"GET POST",
		"GET;",
		"'",
		strings.Repeat("A", 21),
	}
	for _, m := range bad {
		if _, err := normalizeMethods([]string{m}); err == nil {
			t.Fatalf("method %q must be rejected", m)
		}
	}
	got, err := normalizeMethods([]string{" post ", "GET", "get", ""})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "GET POST" {
		t.Fatalf("methods must be upper-cased, de-duplicated and sorted, got %v", got)
	}
	if _, err := normalizeMethods(make([]string, maxAllowedMethods+1)); err == nil {
		t.Fatal("an oversized method list must be rejected")
	}
}

func TestParseConfigRejectsBadRulePolicy(t *testing.T) {
	body := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rules":{"allowedMethods":["GET\nSecRuleEngine Off"]}}]}`
	if _, err := ParseConfig([]byte(body)); err == nil {
		t.Fatal("a method token that could inject directives must fail the config load")
	}
	ok := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rules":{"disableSqli":true,"strict":true,"allowedMethods":["GET","POST"]}}]}`
	cfg, err := ParseConfig([]byte(ok))
	if err != nil {
		t.Fatalf("a valid rule policy was rejected: %v", err)
	}
	p, err := cfg.Sites[0].enginePolicy(ModeDetection)
	if err != nil {
		t.Fatal(err)
	}
	if !p.DisableSQLi || !p.Strict || p.AllowedMethods != "GET POST" {
		t.Fatalf("policy did not round-trip: %+v", p)
	}
}

func TestSitesWithEquivalentPoliciesShareOneEngine(t *testing.T) {
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	// Same effective policy expressed in a different order/case must collapse to
	// one compiled instance rather than paying for a second full ruleset.
	cfg := Config{Sites: []SiteConfig{
		{Host: "a.example", Upstream: "http://127.0.0.1:1", Rules: &RulePolicy{AllowedMethods: []string{"GET", "post"}}},
		{Host: "b.example", Upstream: "http://127.0.0.1:1", Rules: &RulePolicy{AllowedMethods: []string{"POST", "get"}}},
	}}
	rt, err := NewRouter(cfg, engine, ModeBlock, "")
	if err != nil {
		t.Fatal(err)
	}
	// One for the base policy the sites do not use, one shared by both sites.
	if rt.engines != 2 {
		t.Fatalf("equivalent policies must share one engine, got %d compiled", rt.engines)
	}
}
