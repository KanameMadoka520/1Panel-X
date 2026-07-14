package weblog

import (
	"strings"
	"testing"
	"time"
)

func TestParseLineStandardMain(t *testing.T) {
	line := `1.2.3.4 - alice [13/Jul/2026:18:30:00 +0000] "GET /index.html?a=1 HTTP/1.1" 200 1234 "https://ref.example/x" "Mozilla/5.0 (X11)" "9.9.9.9"`
	e, ok := ParseLine(line)
	if !ok {
		t.Fatal("standard main line should parse")
	}
	if e.IP != "1.2.3.4" || e.Method != "GET" || e.URI != "/index.html?a=1" || e.Proto != "HTTP/1.1" {
		t.Fatalf("request fields wrong: %+v", e)
	}
	if e.Status != 200 || e.Bytes != 1234 {
		t.Fatalf("status/bytes wrong: %+v", e)
	}
	if e.Referer != "https://ref.example/x" || e.UserAgent != "Mozilla/5.0 (X11)" || e.XForwardedFor != "9.9.9.9" {
		t.Fatalf("quoted fields wrong: %+v", e)
	}
	if !e.Time.Equal(time.Date(2026, 7, 13, 18, 30, 0, 0, time.UTC)) {
		t.Fatalf("time wrong: %v", e.Time)
	}
}

func TestParseLineBareMainNoRefererUA(t *testing.T) {
	line := `10.0.0.1 - - [01/Jan/2026:00:00:01 +0000] "POST /api HTTP/2.0" 500 0`
	e, ok := ParseLine(line)
	if !ok {
		t.Fatal("bare main line should parse")
	}
	if e.IP != "10.0.0.1" || e.Method != "POST" || e.Status != 500 || e.Bytes != 0 {
		t.Fatalf("fields wrong: %+v", e)
	}
	if e.Referer != "" || e.UserAgent != "" {
		t.Fatalf("optional fields should be empty: %+v", e)
	}
}

func TestParseLineBytesDash(t *testing.T) {
	line := `1.1.1.1 - - [01/Jan/2026:00:00:01 +0000] "GET / HTTP/1.1" 304 -`
	e, ok := ParseLine(line)
	if !ok || e.Bytes != 0 || e.Status != 304 {
		t.Fatalf("dash bytes should be 0: %+v ok=%v", e, ok)
	}
}

func TestParseLineMalformedRejected(t *testing.T) {
	for _, bad := range []string{
		"",
		"garbage not a log line",
		`1.2.3.4 - - 13/Jul/2026:18:30:00 "GET / HTTP/1.1" 200 1`,        // missing [brackets]
		`1.2.3.4 - - [bad-time] "GET / HTTP/1.1" 200 1`,                  // unparseable time
		`1.2.3.4 - - [13/Jul/2026:18:30:00 +0000] "GET / HTTP/1.1" xx 1`, // non-numeric status
	} {
		if _, ok := ParseLine(bad); ok {
			t.Errorf("malformed line should be rejected: %q", bad)
		}
	}
}

// M1: control characters / CRLF injected via a forged field are stripped.
func TestParseLineStripsControlChars(t *testing.T) {
	line := "5.5.5.5 - - [13/Jul/2026:18:30:00 +0000] \"GET /a HTTP/1.1\" 200 1 \"-\" \"Eviluser-Agent\x00\x1b[3m\" \"-\""
	e, ok := ParseLine(line)
	if !ok {
		t.Fatal("should parse")
	}
	if strings.ContainsAny(e.UserAgent, "\x00\x1b") {
		t.Fatalf("control chars not stripped: %q", e.UserAgent)
	}
	if e.UserAgent != "Eviluser-Agent[3m" {
		t.Fatalf("unexpected cleaned UA: %q", e.UserAgent)
	}
}

// M2: an oversized line does not hang or explode; it is bounded then parsed/skipped.
func TestParseLineOversizedBounded(t *testing.T) {
	huge := "1.2.3.4 - - [13/Jul/2026:18:30:00 +0000] \"GET /" + strings.Repeat("a", 100000) + " HTTP/1.1\" 200 1"
	e, ok := ParseLine(huge)
	// Either parses with a bounded URI or is skipped — but never a giant field.
	if ok && len(e.URI) > maxFieldLen {
		t.Fatalf("URI not bounded: %d", len(e.URI))
	}
}

func fixtureEntries(t *testing.T) []AccessEntry {
	t.Helper()
	lines := []string{
		`1.1.1.1 - - [13/Jul/2026:10:00:05 +0000] "GET /a HTTP/1.1" 200 100 "-" "UA1" "-"`,
		`1.1.1.1 - - [13/Jul/2026:10:00:30 +0000] "GET /a HTTP/1.1" 200 100 "-" "UA1" "-"`,
		`2.2.2.2 - - [13/Jul/2026:10:00:45 +0000] "GET /b HTTP/1.1" 404 50 "https://r1" "UA2" "-"`,
		`3.3.3.3 - - [13/Jul/2026:10:01:10 +0000] "GET /a HTTP/1.1" 500 10 "https://r1" "UA3" "-"`,
	}
	var out []AccessEntry
	for _, l := range lines {
		e, ok := ParseLine(l)
		if !ok {
			t.Fatalf("fixture line failed to parse: %s", l)
		}
		out = append(out, e)
	}
	return out
}

func TestAggregateBucketsAndStatusClasses(t *testing.T) {
	buckets, _ := Aggregate(fixtureEntries(t), time.Minute, 10)
	if len(buckets) != 2 {
		t.Fatalf("want 2 minute-buckets, got %d", len(buckets))
	}
	// bucket 10:00 — 3 requests, 2 distinct IPs, 2xx=2, 4xx=1
	b0 := buckets[0]
	if b0.Pv != 3 || b0.Uv != 2 || b0.Status2xx != 2 || b0.Status4xx != 1 {
		t.Fatalf("bucket0 wrong: %+v", b0)
	}
	if b0.Bytes != 250 {
		t.Fatalf("bucket0 bytes: %d", b0.Bytes)
	}
	// bucket 10:01 — 1 request, 1 IP, 5xx=1
	b1 := buckets[1]
	if b1.Pv != 1 || b1.Uv != 1 || b1.Status5xx != 1 {
		t.Fatalf("bucket1 wrong: %+v", b1)
	}
	if !b0.Time.Before(b1.Time) {
		t.Fatal("buckets must be sorted ascending by time")
	}
}

func TestAggregateRankings(t *testing.T) {
	_, ranks := Aggregate(fixtureEntries(t), time.Minute, 2)
	get := func(kind string) []RankItem {
		var r []RankItem
		for _, it := range ranks {
			if it.Kind == kind {
				r = append(r, it)
			}
		}
		return r
	}
	uri := get(RankURI)
	if len(uri) == 0 || uri[0].Key != "/a" || uri[0].Count != 3 {
		t.Fatalf("top uri should be /a x3: %+v", uri)
	}
	ip := get(RankIP)
	if ip[0].Key != "1.1.1.1" || ip[0].Count != 2 {
		t.Fatalf("top ip should be 1.1.1.1 x2: %+v", ip)
	}
	// topN=2 must cap each kind
	if len(get(RankStatus)) > 2 {
		t.Fatalf("status rank not capped to topN: %+v", get(RankStatus))
	}
	ref := get(RankReferer)
	if ref[0].Key != "https://r1" || ref[0].Count != 2 {
		t.Fatalf("top referer should be https://r1 x2: %+v", ref)
	}
}

func TestAggregateEmpty(t *testing.T) {
	buckets, ranks := Aggregate(nil, time.Minute, 10)
	if len(buckets) != 0 || len(ranks) != 0 {
		t.Fatalf("empty input must yield empty output: %d buckets, %d ranks", len(buckets), len(ranks))
	}
}

func at(hour, min int, end int64) LineAt {
	return LineAt{
		Entry: AccessEntry{Time: time.Date(2026, 7, 13, hour, min, 0, 0, time.UTC), IP: "1.1.1.1"},
		End:   end,
	}
}

func TestPartitionClosedFinalizesOnlyClosedPrefix(t *testing.T) {
	now := time.Date(2026, 7, 13, 11, 30, 0, 0, time.UTC) // open bucket = 11:00
	lines := []LineAt{
		at(10, 0, 100),  // closed
		at(10, 30, 200), // closed
		at(11, 5, 300),  // open — stop here
		at(11, 10, 400), // held back
	}
	closed, advanceTo, ok := PartitionClosed(lines, now, time.Hour)
	if !ok || advanceTo != 200 || len(closed) != 2 {
		t.Fatalf("want 2 closed + advanceTo=200 + ok, got closed=%d advanceTo=%d ok=%v", len(closed), advanceTo, ok)
	}
}

func TestPartitionClosedNoneReady(t *testing.T) {
	now := time.Date(2026, 7, 13, 11, 30, 0, 0, time.UTC)
	lines := []LineAt{at(11, 5, 100), at(11, 20, 200)} // all in the open bucket
	closed, advanceTo, ok := PartitionClosed(lines, now, time.Hour)
	if ok || advanceTo != 0 || len(closed) != 0 {
		t.Fatalf("nothing should finalize: closed=%d advanceTo=%d ok=%v", len(closed), advanceTo, ok)
	}
}

// The prefix rule holds back everything after the first open-bucket line, even a
// later line that is itself in a closed bucket — so the cursor never skips an
// un-finalized line when the log is written slightly out of order.
func TestPartitionClosedStopsAtFirstOpenEvenIfLaterLineClosed(t *testing.T) {
	now := time.Date(2026, 7, 13, 11, 30, 0, 0, time.UTC)
	lines := []LineAt{
		at(10, 0, 100),  // closed
		at(11, 5, 200),  // open — stop
		at(10, 40, 300), // closed but out of order → held back
	}
	closed, advanceTo, ok := PartitionClosed(lines, now, time.Hour)
	if !ok || advanceTo != 100 || len(closed) != 1 {
		t.Fatalf("want only first line finalized: closed=%d advanceTo=%d ok=%v", len(closed), advanceTo, ok)
	}
}

func TestRegionRanks(t *testing.T) {
	resolve := func(ip string) string {
		switch ip {
		case "1.1.1.1", "1.1.1.2":
			return "CN"
		case "2.2.2.2":
			return "US"
		default:
			return "" // unknown → skipped
		}
	}
	entries := []AccessEntry{
		{IP: "1.1.1.1"}, {IP: "1.1.1.1"}, {IP: "1.1.1.2"}, // CN x3
		{IP: "2.2.2.2"}, // US x1
		{IP: "9.9.9.9"}, // unknown → skipped
		{IP: ""},        // no IP → skipped
	}
	ranks := RegionRanks(entries, resolve, 10)
	if len(ranks) != 2 || ranks[0].Kind != RankRegion || ranks[0].Key != "CN" || ranks[0].Count != 3 {
		t.Fatalf("top region should be CN x3: %+v", ranks)
	}
	if ranks[1].Key != "US" || ranks[1].Count != 1 {
		t.Fatalf("second region should be US x1: %+v", ranks)
	}
	// topN caps
	if capped := RegionRanks(entries, resolve, 1); len(capped) != 1 || capped[0].Key != "CN" {
		t.Fatalf("topN=1 should keep only CN: %+v", capped)
	}
	// nil resolver (no mmdb) degrades to no region data
	if RegionRanks(entries, nil, 10) != nil {
		t.Fatal("nil resolver must return nil")
	}
}
