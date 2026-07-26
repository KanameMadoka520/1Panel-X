package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readJournal parses every JSON line the gateway wrote.
func readJournal(t *testing.T, path string) []EnforcementEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []EnforcementEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e EnforcementEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("journal line is not valid JSON: %q: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}

func newJournalFixture(t *testing.T) (*EventJournal, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.log")
	j := NewEventJournal(path)
	if j == nil {
		t.Fatal("journal should open in a writable temp dir")
	}
	t.Cleanup(func() { _ = j.Close() })
	return j, path
}

func TestJournalRecordsDeniedIPWithStableID(t *testing.T) {
	journal, path := newJournalFixture(t)

	acl, err := newIPACL(nil, []string{"203.0.113.5"})
	if err != nil {
		t.Fatal(err)
	}
	reached := false
	h := buildGateway(t, ModeBlock, 1<<20, &reached).
		WithRealIPHeader("X-Real-IP").
		WithIPACL(acl).
		WithSite(siteRef{WebsiteID: 42, Host: "a.example"}).
		WithJournal(journal)

	if code := serveFrom(h, "203.0.113.5:1234", "GET", "/secret?x=1", "").Code; code != 403 {
		t.Fatalf("denied IP should get 403, got %d", code)
	}
	if reached {
		t.Fatal("denied IP must not reach upstream")
	}

	events := readJournal(t, path)
	if len(events) != 1 {
		t.Fatalf("expected exactly one record, got %d: %+v", len(events), events)
	}
	e := events[0]
	if e.Kind != EventACLDeny || e.Action != "blocked" {
		t.Fatalf("unexpected kind/action: %+v", e)
	}
	if e.WebsiteID != 42 || e.Host != "a.example" {
		t.Fatalf("decision must be attributed to its site: %+v", e)
	}
	if e.ClientIP != "203.0.113.5" {
		t.Fatalf("client IP must be the evaluated one without a port: %q", e.ClientIP)
	}
	if e.URI != "/secret?x=1" || e.Method != "GET" {
		t.Fatalf("request detail missing: %+v", e)
	}
	if e.ID == "" {
		t.Fatal("every record needs an id so re-reading the journal is idempotent")
	}
	if e.Time.IsZero() {
		t.Fatal("every record needs a timestamp")
	}
}

func TestJournalIDsAreUniquePerLine(t *testing.T) {
	journal, path := newJournalFixture(t)
	for i := 0; i < 5; i++ {
		journal.Record(EnforcementEvent{Kind: EventACLDeny, Action: "blocked"})
	}
	events := readJournal(t, path)
	if len(events) != 5 {
		t.Fatalf("expected 5 records, got %d", len(events))
	}
	seen := map[string]bool{}
	for _, e := range events {
		if seen[e.ID] {
			t.Fatalf("duplicate id %q would collapse distinct events on ingest", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestJournalRecordsUnknownHostWithoutSiteAttribution(t *testing.T) {
	journal, path := newJournalFixture(t)
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sites: []SiteConfig{{WebsiteID: 1, Host: "a.example", Upstream: "http://127.0.0.1:8080"}}}
	rt, err := NewRouterWithJournal(cfg, engine, ModeBlock, "", journal)
	if err != nil {
		t.Fatal(err)
	}

	if code := serve(rt, "GET", "http://unknown.example/", "").Code; code != 403 {
		t.Fatal("unknown Host must be denied")
	}
	events := readJournal(t, path)
	if len(events) != 1 || events[0].Kind != EventUnknownHost {
		t.Fatalf("unexpected records: %+v", events)
	}
	// The request matched no site, so it must not be attributed to one.
	if events[0].WebsiteID != 0 {
		t.Fatalf("unknown-host records must stay unattributed, got websiteId=%d", events[0].WebsiteID)
	}
	if events[0].Host != "unknown.example" {
		t.Fatalf("the forged Host itself must be recorded, got %q", events[0].Host)
	}
}

func TestJournalRecordsOversizeBody(t *testing.T) {
	journal, path := newJournalFixture(t)
	reached := false
	h := buildGateway(t, ModeBlock, 16, &reached).WithJournal(journal)

	req := httptest.NewRequest("POST", "/upload", strings.NewReader(strings.Repeat("A", 64)))
	req.ContentLength = 64
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 413 {
		t.Fatalf("oversize body should be 413, got %d", rec.Code)
	}

	events := readJournal(t, path)
	if len(events) != 1 || events[0].Kind != EventOversize || events[0].Action != "blocked" {
		t.Fatalf("unexpected records: %+v", events)
	}
}

func TestJournalTruncatesAndCannotForgeExtraLines(t *testing.T) {
	journal, path := newJournalFixture(t)
	// A crafted URI carrying newlines must not be able to inject a second record.
	journal.Record(EnforcementEvent{
		Kind:   EventACLDeny,
		Action: "blocked",
		URI:    "/a\n{\"id\":\"forged\",\"kind\":\"acl-deny\"}\n",
		Host:   strings.Repeat("h", maxEventFieldBytes*2),
	})
	events := readJournal(t, path)
	if len(events) != 1 {
		t.Fatalf("a newline in a field must not forge extra journal lines, got %d records", len(events))
	}
	if len(events[0].Host) != maxEventFieldBytes {
		t.Fatalf("oversized field must be truncated, got %d bytes", len(events[0].Host))
	}
	if !strings.Contains(events[0].URI, "forged") {
		t.Fatal("the payload should survive as DATA inside the single record")
	}
}

func TestNilJournalIsSafe(t *testing.T) {
	var journal *EventJournal
	journal.Record(EnforcementEvent{Kind: EventACLDeny})
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	// An unwritable path must degrade to a no-op rather than failing construction:
	// losing visibility is bad, refusing traffic over a log file would be worse.
	if j := NewEventJournal(filepath.Join(t.TempDir(), "no-such-dir", "events.log")); j != nil {
		t.Fatal("an unopenable journal should degrade to nil")
	}
	if j := NewEventJournal("  "); j != nil {
		t.Fatal("an empty path should disable the journal")
	}
}
