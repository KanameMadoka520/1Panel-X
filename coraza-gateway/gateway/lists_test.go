package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func buildListGateway(t *testing.T, rules []ListRule, groups []IPGroup, reached *bool) (*Handler, string) {
	t.Helper()
	journal, path := newJournalFixture(t)
	m, err := newListMatcher(rules, groups)
	if err != nil {
		t.Fatalf("compile lists: %v", err)
	}
	h := buildGateway(t, ModeBlock, 1<<20, reached).
		WithRealIPHeader("X-Real-IP").
		WithLists(m).
		WithSite(siteRef{WebsiteID: 1, Host: "a.example"}).
		WithJournal(journal)
	return h, path
}

func request(h http.Handler, ip, target, ua string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", target, strings.NewReader(""))
	req.Header.Set("X-Real-IP", ip)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestURLDenyListBlocksMatchingRequests(t *testing.T) {
	var reached bool
	h, path := buildListGateway(t, []ListRule{
		{List: ListDeny, Target: ListTargetURL, Match: ListMatchContains, Pattern: "/wp-admin"},
	}, nil, &reached)

	if code := request(h, "203.0.113.1", "/wp-admin/setup.php", "").Code; code != http.StatusForbidden {
		t.Fatalf("a denied URL must be refused, got %d", code)
	}
	if reached {
		t.Fatal("a denied URL must never reach the origin")
	}
	if code := request(h, "203.0.113.1", "/index.html", "").Code; code != http.StatusOK {
		t.Fatalf("an unrelated URL must pass, got %d", code)
	}

	events := readJournal(t, path)
	if len(events) != 1 || events[0].Kind != EventACLDeny {
		t.Fatalf("the block must be recorded, got %+v", events)
	}
	if !strings.Contains(events[0].Rule, "url") {
		t.Fatalf("the record must name what matched, got %q", events[0].Rule)
	}
}

func TestUserAgentListsMatchCaseInsensitively(t *testing.T) {
	var reached bool
	h, _ := buildListGateway(t, []ListRule{
		{List: ListDeny, Target: ListTargetUserAgent, Match: ListMatchContains, Pattern: "BadBot"},
	}, nil, &reached)

	if code := request(h, "203.0.113.2", "/", "Mozilla/5.0 badbot/2.1").Code; code != http.StatusForbidden {
		t.Fatalf("a denied User-Agent must be refused regardless of case, got %d", code)
	}
	if code := request(h, "203.0.113.2", "/", "Mozilla/5.0 GoodBot/1.0").Code; code != http.StatusOK {
		t.Fatalf("an unrelated User-Agent must pass, got %d", code)
	}
}

func TestRegexListEntryMatches(t *testing.T) {
	var reached bool
	h, _ := buildListGateway(t, []ListRule{
		{List: ListDeny, Target: ListTargetURL, Match: ListMatchRegex, Pattern: `^/api/v[0-9]+/internal`},
	}, nil, &reached)

	if code := request(h, "203.0.113.3", "/api/v2/internal/metrics", "").Code; code != http.StatusForbidden {
		t.Fatalf("regex entry should match, got %d", code)
	}
	if code := request(h, "203.0.113.3", "/api/public/internal", "").Code; code != http.StatusOK {
		t.Fatalf("regex entry should not over-match, got %d", code)
	}
}

func TestIPGroupEntryMatchesEveryMember(t *testing.T) {
	var reached bool
	h, _ := buildListGateway(t,
		[]ListRule{{List: ListDeny, Target: ListTargetIPGroup, Pattern: "scanners"}},
		[]IPGroup{{Name: "scanners", Entries: []string{"203.0.113.0/24", "198.51.100.7"}}},
		&reached)

	for _, ip := range []string{"203.0.113.9", "203.0.113.250", "198.51.100.7"} {
		if code := request(h, ip, "/", "").Code; code != http.StatusForbidden {
			t.Fatalf("%s is in the group and must be refused, got %d", ip, code)
		}
	}
	if code := request(h, "192.0.2.1", "/", "").Code; code != http.StatusOK {
		t.Fatalf("an address outside the group must pass, got %d", code)
	}
}

func TestAllowListExemptsFromInspectionButDenyStillWins(t *testing.T) {
	var reached bool
	h, _ := buildListGateway(t, []ListRule{
		{List: ListAllow, Target: ListTargetIP, Pattern: "203.0.113.10"},
	}, nil, &reached)

	// An allow entry exempts the client from CRS inspection.
	reached = false
	if code := request(h, "203.0.113.10", sqliTarget, "").Code; code != http.StatusOK || !reached {
		t.Fatalf("an allow-listed client must bypass inspection, got %d reached=%v", code, reached)
	}
	// A client that is not allow-listed is still inspected.
	if code := request(h, "203.0.113.11", sqliTarget, "").Code; code != http.StatusForbidden {
		t.Fatalf("an ordinary client must still be inspected, got %d", code)
	}

	// The same client on both lists: the explicit refusal wins.
	both, _ := buildListGateway(t, []ListRule{
		{List: ListAllow, Target: ListTargetIP, Pattern: "203.0.113.10"},
		{List: ListDeny, Target: ListTargetIP, Pattern: "203.0.113.10"},
	}, nil, &reached)
	if code := request(both, "203.0.113.10", "/", "").Code; code != http.StatusForbidden {
		t.Fatalf("an explicit deny must outrank an explicit allow, got %d", code)
	}
}

func TestPanelListDenyOutranksSiteAllow(t *testing.T) {
	var reached bool
	journal, _ := newJournalFixture(t)
	m, err := newListMatcher([]ListRule{{List: ListDeny, Target: ListTargetIP, Pattern: "203.0.113.20"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	acl, err := newIPACL([]string{"203.0.113.20"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := buildGateway(t, ModeBlock, 1<<20, &reached).
		WithRealIPHeader("X-Real-IP").
		WithLists(m).
		WithIPACL(acl).
		WithJournal(journal)

	// A site-level allow must not re-admit a client the panel refused globally.
	if code := request(h, "203.0.113.20", "/", "").Code; code != http.StatusForbidden {
		t.Fatalf("the panel-wide deny must win over a site allow, got %d", code)
	}
	if reached {
		t.Fatal("the origin must not be reached")
	}
}

func TestListCompilationRejectsBadEntries(t *testing.T) {
	cases := map[string]struct {
		rules  []ListRule
		groups []IPGroup
	}{
		"unknown target":       {rules: []ListRule{{List: ListDeny, Target: "nope", Pattern: "x"}}},
		"unknown list":         {rules: []ListRule{{List: "maybe", Target: ListTargetURL, Pattern: "x"}}},
		"unknown match":        {rules: []ListRule{{List: ListDeny, Target: ListTargetURL, Match: "fuzzy", Pattern: "x"}}},
		"empty pattern":        {rules: []ListRule{{List: ListDeny, Target: ListTargetURL, Pattern: "   "}}},
		"invalid ip":           {rules: []ListRule{{List: ListDeny, Target: ListTargetIP, Pattern: "999.1.1.1"}}},
		"invalid regex":        {rules: []ListRule{{List: ListDeny, Target: ListTargetURL, Match: ListMatchRegex, Pattern: "a(("}}},
		"unknown ip group":     {rules: []ListRule{{List: ListDeny, Target: ListTargetIPGroup, Pattern: "ghosts"}}},
		"oversized pattern":    {rules: []ListRule{{List: ListDeny, Target: ListTargetURL, Pattern: strings.Repeat("x", maxListPatternBytes+1)}}},
		"duplicate group name": {rules: []ListRule{{List: ListDeny, Target: ListTargetIPGroup, Pattern: "g"}}, groups: []IPGroup{{Name: "g", Entries: []string{"1.2.3.4"}}, {Name: "g"}}},
		"empty group name":     {rules: []ListRule{{List: ListDeny, Target: ListTargetURL, Pattern: "x"}}, groups: []IPGroup{{Name: "  "}}},
		"invalid group member": {rules: []ListRule{{List: ListDeny, Target: ListTargetIPGroup, Pattern: "g"}}, groups: []IPGroup{{Name: "g", Entries: []string{"bogus"}}}},
	}
	for name, c := range cases {
		if _, err := newListMatcher(c.rules, c.groups); err == nil {
			t.Fatalf("%s must be refused at load time, not silently dropped", name)
		}
	}
}

func TestParseConfigRejectsBadLists(t *testing.T) {
	body := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1"}],"lists":[{"list":"deny","target":"url","match":"regex","pattern":"a(("}]}`
	if _, err := ParseConfig([]byte(body)); err == nil {
		t.Fatal("an unusable list must fail the whole config load")
	}
	ok := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1"}],"ipGroups":[{"name":"g","entries":["10.0.0.0/8"]}],"lists":[{"list":"deny","target":"ipgroup","pattern":"g"}]}`
	if _, err := ParseConfig([]byte(ok)); err != nil {
		t.Fatalf("a valid list set was rejected: %v", err)
	}
}

func TestEmptyListSetIsNil(t *testing.T) {
	m, err := newListMatcher(nil, nil)
	if err != nil || m != nil {
		t.Fatalf("no rules should compile to no matcher: %v %v", m, err)
	}
	if !m.empty() {
		t.Fatal("a nil matcher must report empty")
	}
	decision, rule := m.decide(httptest.NewRequest("GET", "/", nil), nil)
	if decision != aclNormal || rule != "" {
		t.Fatal("a nil matcher must not decide anything")
	}
}
