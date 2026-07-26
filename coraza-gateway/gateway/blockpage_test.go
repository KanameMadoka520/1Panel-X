package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func buildBlockPageRouter(t *testing.T, page *BlockPage, reached *bool) http.Handler {
	t.Helper()
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Sites:     []SiteConfig{{Host: "a.example", Upstream: "http://127.0.0.1:1", DenyIPs: []string{"203.0.113.7"}}},
		BlockPage: page,
	}
	rt, err := NewRouter(cfg, engine, ModeBlock, "")
	if err != nil {
		t.Fatal(err)
	}
	rt.handlers["a.example"].(*Handler).upstream = recordingUpstream(reached)
	return rt
}

// The load-bearing assertion: the operator's page is what a refused visitor
// actually sees, with the configured status.
func TestCustomBlockPageIsServed(t *testing.T) {
	var reached bool
	h := buildBlockPageRouter(t, &BlockPage{
		Status: http.StatusNotFound,
		HTML:   "<h1>Nothing here</h1>",
	}, &reached)

	rr := customRequest(h, "GET", "/", nil, "203.0.113.7:1234")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("the configured status must be served, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Nothing here") {
		t.Fatalf("the operator's page must be served, got %q", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Request blocked") {
		t.Fatal("the built-in page must not be served alongside the operator's")
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("the block page must be served as HTML, got %q", ct)
	}
}

// The client address reaches us through the trusted real-IP header. An
// unescaped value there would let whoever can set that header inject markup into
// a page the operator's own visitors are shown.
func TestBlockPagePlaceholdersAreEscaped(t *testing.T) {
	page := newBlockPage(&BlockPage{HTML: `<p>from {{ip}} at {{time}}</p>`})
	out := page.render(`1.2.3.4"><script>alert(1)</script>`, time.Unix(0, 0))

	if strings.Contains(out, "<script>") {
		t.Fatalf("a crafted client address must not inject markup: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("the address must be HTML-escaped: %s", out)
	}
	if strings.Contains(out, "{{time}}") {
		t.Fatalf("the time placeholder must be substituted: %s", out)
	}
}

func TestBlockPageFallsBackToTheBuiltInPage(t *testing.T) {
	var reached bool
	h := buildBlockPageRouter(t, nil, &reached)
	rr := customRequest(h, "GET", "/", nil, "203.0.113.7:1234")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("the default refusal is 403, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Request blocked") {
		t.Fatalf("the built-in page must be served when none is configured, got %q", rr.Body.String())
	}
	// An unknown Host is refused by the router itself and must look identical to
	// a visitor, or the difference becomes a way to enumerate configured sites.
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "not-configured.example"
	unknown := httptest.NewRecorder()
	h.ServeHTTP(unknown, req)
	if unknown.Code != rr.Code || unknown.Body.String() != rr.Body.String() {
		t.Fatalf("an unknown Host must be indistinguishable from a rule hit: %d %q", unknown.Code, unknown.Body.String())
	}
}

func TestBlockPageValidation(t *testing.T) {
	if err := (&BlockPage{Status: 500}).validate(); err == nil {
		t.Fatal("a 5xx would blame the origin for a decision the WAF made and must be refused")
	}
	if err := (&BlockPage{Status: 302}).validate(); err == nil {
		t.Fatal("a redirect status must be refused")
	}
	if err := (&BlockPage{HTML: strings.Repeat("a", maxBlockPageBytes+1)}).validate(); err == nil {
		t.Fatal("an oversized block page must be refused")
	}
	for _, ok := range []int{0, 200, 403, 404} {
		if err := (&BlockPage{Status: ok}).validate(); err != nil {
			t.Fatalf("status %d must be accepted: %v", ok, err)
		}
	}
	body := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1"}],"blockPage":{"status":500}}`
	if _, err := ParseConfig([]byte(body)); err == nil {
		t.Fatal("an invalid block page must fail the whole config load")
	}
}

// An excluded record kind must actually stop being written, and every other kind
// must keep being written — a switch that quietly took the rest down with it
// would be worse than no switch.
func TestExcludedRecordKindsAreNotWritten(t *testing.T) {
	dir := t.TempDir()
	journal := NewEventJournal(dir + "/events.log")
	defer journal.Close()
	journal.SetExcluded(map[EventKind]struct{}{EventACLDeny: {}})

	journal.Record(EnforcementEvent{Kind: EventACLDeny, Action: "blocked"})
	journal.Record(EnforcementEvent{Kind: EventRateLimit, Action: "blocked"})

	lines := readJournal(t, dir+"/events.log")
	if len(lines) != 1 || lines[0].Kind != EventRateLimit {
		t.Fatalf("only the excluded kind may be dropped, got %+v", lines)
	}
	// Clearing the exclusion must resume recording.
	journal.SetExcluded(nil)
	journal.Record(EnforcementEvent{Kind: EventACLDeny, Action: "blocked"})
	if lines := readJournal(t, dir+"/events.log"); len(lines) != 2 {
		t.Fatalf("clearing the exclusion must resume recording, got %d records", len(lines))
	}
}

func TestUnknownRecordKindIsRefused(t *testing.T) {
	if err := (LogSettings{ExcludedKinds: []string{"ratelimits"}}).validate(); err == nil {
		t.Fatal("a misspelled record kind must be refused, not silently ignored")
	}
	if err := (LogSettings{ExcludedKinds: []string{string(EventRateLimit), string(EventRegion)}}).validate(); err != nil {
		t.Fatalf("known kinds must be accepted: %v", err)
	}
}
