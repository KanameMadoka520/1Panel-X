package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// buildCustomGateway wires one site behind the given custom rules, using the
// real Router so the evaluation order under test is the production one.
func buildCustomGateway(t *testing.T, rules []CustomRule, reached *bool, journal *EventJournal) http.Handler {
	t.Helper()
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	cfg := Config{
		Sites:       []SiteConfig{{Host: "a.example", Upstream: "http://127.0.0.1:1"}},
		CustomRules: rules,
	}
	rt, err := NewRouterWithJournal(cfg, engine, ModeBlock, "", journal)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	// Swap the compiled site handler's upstream for a recorder; everything else
	// (lists, custom rules, ACL, enforcer) is exactly what the router built.
	h := rt.handlers["a.example"].(*Handler)
	h.upstream = recordingUpstream(reached)
	return rt
}

func customRequest(h http.Handler, method, target string, headers map[string]string, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = "a.example"
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// The load-bearing assertion: a deny rule actually refuses the request and it
// never reaches the origin, while traffic the rule does not describe passes.
func TestCustomDenyRuleRefusesMatchingRequests(t *testing.T) {
	var reached bool
	h := buildCustomGateway(t, []CustomRule{{
		Name:   "block-admin",
		Action: CustomDeny,
		Conditions: []CustomCondition{
			{Field: FieldPath, Match: ListMatchPrefix, Pattern: "/admin"},
		},
	}}, &reached, nil)

	reached = false
	if code := customRequest(h, "GET", "/admin/login", nil, "").Code; code != http.StatusForbidden {
		t.Fatalf("a matching request must be refused, got %d", code)
	}
	if reached {
		t.Fatal("a refused request must not reach the origin")
	}
	reached = false
	if code := customRequest(h, "GET", "/public/index", nil, "").Code; code != http.StatusOK || !reached {
		t.Fatalf("a non-matching request must pass, got %d reached=%v", code, reached)
	}
}

// Conditions are ANDed. A rule whose conditions only partly hold must not fire —
// the opposite would silently widen every rule an operator writes.
func TestCustomRuleConditionsAreAnded(t *testing.T) {
	var reached bool
	h := buildCustomGateway(t, []CustomRule{{
		Action: CustomDeny,
		Conditions: []CustomCondition{
			{Field: FieldPath, Match: ListMatchPrefix, Pattern: "/api"},
			{Field: FieldMethod, Match: ListMatchExact, Pattern: "POST"},
			{Field: FieldHeader, Name: "X-Tenant", Match: ListMatchExact, Pattern: "acme"},
		},
	}}, &reached, nil)

	reached = false
	if code := customRequest(h, "POST", "/api/orders", map[string]string{"X-Tenant": "acme"}, "").Code; code != http.StatusForbidden {
		t.Fatalf("all conditions hold, the request must be refused, got %d", code)
	}
	for name, call := range map[string]func() int{
		"wrong path": func() int { return customRequest(h, "POST", "/other", map[string]string{"X-Tenant": "acme"}, "").Code },
		"wrong method": func() int {
			return customRequest(h, "GET", "/api/orders", map[string]string{"X-Tenant": "acme"}, "").Code
		},
		"wrong header": func() int {
			return customRequest(h, "POST", "/api/orders", map[string]string{"X-Tenant": "other"}, "").Code
		},
		"no header": func() int { return customRequest(h, "POST", "/api/orders", nil, "").Code },
	} {
		reached = false
		if code := call(); code != http.StatusOK || !reached {
			t.Fatalf("%s: only some conditions hold, the request must pass, got %d reached=%v", name, code, reached)
		}
	}
}

// An allow rule is an operator exemption: it must bypass rule-set inspection and
// still be proxied. This is the assertion that distinguishes it from "do nothing".
func TestCustomAllowRuleBypassesInspection(t *testing.T) {
	var reached bool
	h := buildCustomGateway(t, []CustomRule{{
		Name:       "trust-health-checker",
		Action:     CustomAllow,
		Conditions: []CustomCondition{{Field: FieldHeader, Name: "X-Health", Match: ListMatchExact, Pattern: "probe"}},
	}}, &reached, nil)

	// A payload that would otherwise be blocked passes for the exempted client...
	reached = false
	if code := customRequest(h, "GET", sqliTarget, map[string]string{"X-Health": "probe"}, "").Code; code != http.StatusOK || !reached {
		t.Fatalf("an allow rule must bypass inspection, got %d reached=%v", code, reached)
	}
	// ...and is still blocked for everyone else.
	reached = false
	if code := customRequest(h, "GET", sqliTarget, nil, "").Code; code != http.StatusForbidden {
		t.Fatalf("the same payload must still be blocked without the exemption, got %d", code)
	}
}

// The first matching armed rule wins, and a `log` rule does not stop the scan —
// otherwise switching a rule to "log" would disarm every rule below it.
func TestCustomRuleOrderAndLogAction(t *testing.T) {
	var reached bool
	dir := t.TempDir()
	journal := NewEventJournal(dir + "/events.log")
	defer journal.Close()

	h := buildCustomGateway(t, []CustomRule{
		{Name: "watch", Action: CustomLog, Conditions: []CustomCondition{{Field: FieldPath, Match: ListMatchPrefix, Pattern: "/api"}}},
		{Name: "block", Action: CustomDeny, Conditions: []CustomCondition{{Field: FieldPath, Match: ListMatchPrefix, Pattern: "/api/secret"}}},
	}, &reached, journal)

	// A log-only match records but lets the request through.
	reached = false
	if code := customRequest(h, "GET", "/api/orders", nil, "").Code; code != http.StatusOK || !reached {
		t.Fatalf("a log rule must not refuse the request, got %d reached=%v", code, reached)
	}
	// The armed rule below it still fires.
	reached = false
	if code := customRequest(h, "GET", "/api/secret", nil, "").Code; code != http.StatusForbidden {
		t.Fatalf("a log rule must not disarm the rules below it, got %d", code)
	}

	lines := readJournal(t, dir+"/events.log")
	var observed, blocked int
	for _, e := range lines {
		if e.Kind != EventCustomRule {
			continue
		}
		switch e.Action {
		case "detected":
			observed++
			if !strings.Contains(e.Rule, "watch") {
				t.Fatalf("a detected record must name the rule that fired: %q", e.Rule)
			}
		case "blocked":
			blocked++
			if !strings.Contains(e.Rule, "block") {
				t.Fatalf("a blocked record must name the rule that fired: %q", e.Rule)
			}
		}
	}
	if observed != 2 || blocked != 1 {
		t.Fatalf("expected 2 detected and 1 blocked custom-rule records, got %d and %d", observed, blocked)
	}
}

func TestCustomRuleNegationAndIPCondition(t *testing.T) {
	var reached bool
	h := buildCustomGateway(t, []CustomRule{{
		Name:   "office-only-admin",
		Action: CustomDeny,
		Conditions: []CustomCondition{
			{Field: FieldPath, Match: ListMatchPrefix, Pattern: "/admin"},
			{Field: FieldIP, Pattern: "203.0.113.0/24", Negate: true},
		},
	}}, &reached, nil)

	reached = false
	if code := customRequest(h, "GET", "/admin", nil, "198.51.100.7:1234").Code; code != http.StatusForbidden {
		t.Fatalf("an address outside the office range must be refused, got %d", code)
	}
	reached = false
	if code := customRequest(h, "GET", "/admin", nil, "203.0.113.9:1234").Code; code != http.StatusOK || !reached {
		t.Fatalf("an address inside the office range must pass, got %d reached=%v", code, reached)
	}
}

// The panel-wide lists are the primary control, so a custom allow must not be
// able to re-admit a client the panel refused globally.
func TestPanelDenyOutranksCustomAllow(t *testing.T) {
	var reached bool
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Sites: []SiteConfig{{Host: "a.example", Upstream: "http://127.0.0.1:1"}},
		Lists: []ListRule{{List: ListDeny, Target: ListTargetIP, Pattern: "198.51.100.7"}},
		CustomRules: []CustomRule{{
			Action:     CustomAllow,
			Conditions: []CustomCondition{{Field: FieldPath, Match: ListMatchPrefix, Pattern: "/"}},
		}},
	}
	rt, err := NewRouter(cfg, engine, ModeBlock, "")
	if err != nil {
		t.Fatal(err)
	}
	rt.handlers["a.example"].(*Handler).upstream = recordingUpstream(&reached)

	reached = false
	if code := customRequest(rt, "GET", "/", nil, "198.51.100.7:1234").Code; code != http.StatusForbidden {
		t.Fatalf("a panel deny must outrank a custom allow, got %d", code)
	}
	if reached {
		t.Fatal("a panel-denied client must never reach the origin")
	}
}

func TestCustomRuleValidation(t *testing.T) {
	bad := map[string]CustomRule{
		"no conditions": {Action: CustomDeny},
		"unknown action": {Action: "drop",
			Conditions: []CustomCondition{{Field: FieldPath, Pattern: "/x"}}},
		"unknown field": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: "body", Pattern: "/x"}}},
		"unknown match": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldPath, Match: "glob", Pattern: "/x"}}},
		"empty pattern": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldPath, Pattern: "   "}}},
		"bad regex": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldPath, Match: ListMatchRegex, Pattern: "("}}},
		"header without name": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldHeader, Pattern: "x"}}},
		"bad header name": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldHeader, Name: "X Tenant: evil", Pattern: "x"}}},
		"name on a field that has none": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldPath, Name: "nope", Pattern: "/x"}}},
		"match type on an address": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldIP, Match: ListMatchPrefix, Pattern: "10.0.0.0/8"}}},
		"bad address": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldIP, Pattern: "not-an-address"}}},
		"oversized pattern": {Action: CustomDeny,
			Conditions: []CustomCondition{{Field: FieldPath, Pattern: strings.Repeat("a", maxCustomPatternBytes+1)}}},
		"too many conditions": {Action: CustomDeny,
			Conditions: make([]CustomCondition, maxCustomConditions+1)},
	}
	for name, rule := range bad {
		if _, err := newCustomMatcher([]CustomRule{rule}); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
	if _, err := newCustomMatcher(make([]CustomRule, maxCustomRules+1)); err == nil {
		t.Fatal("an oversized rule list must be rejected")
	}
	// An invalid rule must fail the WHOLE config: enforcing the rest of the
	// operator's policy while silently dropping one rule is worse than refusing.
	body := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1"}],
	          "customRules":[{"action":"deny","conditions":[{"field":"path","pattern":"/ok"}]},
	                         {"action":"deny","conditions":[]}]}`
	if _, err := ParseConfig([]byte(body)); err == nil {
		t.Fatal("one invalid custom rule must fail the whole config load")
	}
}
