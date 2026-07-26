package service

import (
	"strings"
	"testing"
	"time"
)

func resolveNothing(string) uint { return 0 }

func TestBuildBlockRecordsParsesTheJournal(t *testing.T) {
	lines := []string{
		`{"id":"abc-1","time":"2026-07-26T10:00:00Z","kind":"acl-deny","websiteId":7,"host":"a.example","clientIp":"203.0.113.7","method":"GET","uri":"/x","rule":"deny:ip","action":"blocked"}` + "\n",
		`{"id":"abc-2","time":"2026-07-26T10:00:01Z","kind":"challenge","websiteId":7,"host":"a.example","clientIp":"203.0.113.7","method":"GET","uri":"/manage","rule":"custom:captcha:guard","action":"challenged"}` + "\n",
	}
	got := buildBlockRecords(lines, resolveNothing)
	if len(got) != 2 {
		t.Fatalf("both records must be ingested, got %d", len(got))
	}
	if got[0].RecordID != "abc-1" || got[0].Kind != "acl-deny" || got[0].WebsiteID != 7 {
		t.Fatalf("record did not round-trip: %+v", got[0])
	}
	// The action must be what actually happened. A challenge is neither a block
	// nor a pass, and flattening it into either would misreport the outcome.
	if got[1].Action != "challenged" {
		t.Fatalf("a challenge must be recorded as challenged, got %q", got[1].Action)
	}
	if !got[0].Time.Equal(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("the gateway's timestamp must be kept: %v", got[0].Time)
	}
}

// One malformed line must not stop the rest from being ingested — a journal that
// stops on the first bad byte loses everything written after it.
func TestBuildBlockRecordsSkipsOnlyTheBadLines(t *testing.T) {
	lines := []string{
		"{not json\n",
		"\n",
		`{"id":"","kind":"acl-deny"}` + "\n",
		`{"id":"ok-1","kind":"ratelimit","action":"blocked"}` + "\n",
	}
	got := buildBlockRecords(lines, resolveNothing)
	if len(got) != 1 || got[0].RecordID != "ok-1" {
		t.Fatalf("only the usable line may be ingested, got %+v", got)
	}
	// A record with no timestamp still lands, stamped on arrival, rather than
	// being dropped or stored at the zero time where no query would find it.
	if got[0].Time.IsZero() {
		t.Fatal("a record with no timestamp must be stamped, not stored at zero")
	}
}

// Attacker-influenced fields are sanitized on this side too. The gateway already
// truncates them, but this is the side that writes to the database and renders in
// a browser, so it does not get to assume the other side did its job.
func TestBuildBlockRecordsSanitizesAndBoundsFields(t *testing.T) {
	long := strings.Repeat("a", 4000)
	lines := []string{
		`{"id":"x","kind":"acl-deny","uri":"/x` + "\\u0000\\u001b" + `[31m","host":"` + long + `","rule":"` + long + `"}` + "\n",
	}
	got := buildBlockRecords(lines, resolveNothing)
	if len(got) != 1 {
		t.Fatalf("the record must still be ingested, got %d", len(got))
	}
	r := got[0]
	if strings.ContainsAny(r.URI, "\x00\x1b") {
		t.Fatalf("control characters must be stripped: %q", r.URI)
	}
	if len(r.Host) > 253 {
		t.Fatalf("host must be bounded to its column, got %d bytes", len(r.Host))
	}
	if len(r.Rule) > 128 {
		t.Fatalf("rule must be bounded to its column, got %d bytes", len(r.Rule))
	}
}

// A record with no website id is resolved by host. One that resolves to nothing
// is still kept: an unknown Host belongs to no website, and dropping those would
// hide exactly the requests worth looking at.
func TestBlockRecordsWithoutAWebsiteAreStillKept(t *testing.T) {
	lines := []string{
		`{"id":"h1","kind":"unknown-host","host":"forged.example","action":"blocked"}` + "\n",
		`{"id":"h2","kind":"acl-deny","host":"known.example","action":"blocked"}` + "\n",
	}
	got := buildBlockRecords(lines, func(host string) uint {
		if host == "known.example" {
			return 42
		}
		return 0
	})
	if len(got) != 2 {
		t.Fatalf("both records must be kept, got %d", len(got))
	}
	if got[0].WebsiteID != 0 {
		t.Fatalf("an unattributable record must not be given an arbitrary website: %+v", got[0])
	}
	if got[1].WebsiteID != 42 {
		t.Fatalf("a resolvable host must be attributed: %+v", got[1])
	}
}
