package wafaudit

import (
	"encoding/json"
	"strings"
	"testing"
)

// A faithful reduction of a real coraza-gateway JSON audit record (see the
// captured schema): two messages — a specific SQLi rule (942100, critical,
// attack-sqli) and the generic anomaly-scoring blocking rule (949110).
const realRecord = `{"transaction":{"unix_timestamp":1784146630877427358,"id":"EoWvNRJCWOYXqpgngop","client_ip":"192.0.2.1","client_port":1234,"server_id":"example.com","request":{"method":"GET","uri":"/?id=1%27%20OR%20%271%27%3D%271","headers":{"host":["example.com"]}},"is_interrupted":true},"messages":[{"error_message":"[client \"192.0.2.1\"] Coraza: Warning. SQL Injection Attack Detected via libinjection [file \"@owasp_crs/REQUEST-942-APPLICATION-ATTACK-SQLI.conf\"] [id \"942100\"] [msg \"SQL Injection Attack Detected via libinjection\"] [data \"Matched Data: s&sos found within ARGS:id: 1' OR '1'='1\"] [severity \"critical\"] [tag \"attack-sqli\"] [tag \"paranoia-level/1\"]"},{"error_message":"[client \"192.0.2.1\"] Coraza: Access denied (phase 2). [id \"949110\"] [msg \"Inbound Anomaly Score Exceeded (Total Score: 15)\"] [data \"\"] [severity \"unknown\"] [tag \"anomaly-evaluation\"]"}]}`

func TestParseRealRecord(t *testing.T) {
	ev, ok := ParseRecord([]byte(realRecord))
	if !ok {
		t.Fatal("real record should parse")
	}
	if ev.Host != "example.com" || ev.SourceIP != "192.0.2.1" || ev.Method != "GET" {
		t.Fatalf("request fields wrong: %+v", ev)
	}
	// The specific SQLi rule must win over the generic anomaly-scoring rule.
	if ev.RuleID != 942100 {
		t.Fatalf("primary rule should be 942100 (SQLi), got %d", ev.RuleID)
	}
	if ev.Category != "sqli" || ev.Severity != "critical" {
		t.Fatalf("category/severity wrong: cat=%q sev=%q", ev.Category, ev.Severity)
	}
	if !ev.Interrupted || ev.HitCount != 2 {
		t.Fatalf("interrupted/hitcount wrong: interrupted=%v hits=%d", ev.Interrupted, ev.HitCount)
	}
	if !strings.Contains(ev.MatchedData, "Matched Data") {
		t.Fatalf("matched-data snippet missing: %q", ev.MatchedData)
	}
	if ev.Time.IsZero() {
		t.Fatal("timestamp not parsed")
	}
}

// W6: control characters injected via the (attacker-controlled) matched-data
// snippet are stripped before the event leaves the parser. The NUL/ESC bytes
// are built from numeric byte literals (never a source escape) and marshalled —
// exactly what a decoded coraza audit field would contain.
func TestParseStripsControlChars(t *testing.T) {
	dirtyData := "Matched: evil" + string([]byte{0, 27}) + "[3mPWN"
	rec := map[string]any{
		"transaction": map[string]any{
			"unix_timestamp": int64(1784146630877427358),
			"client_ip":      "5.5.5.5",
			"server_id":      "x",
			"is_interrupted": false,
			"request": map[string]any{
				"method":  "GET",
				"uri":     "/a",
				"headers": map[string][]string{"host": {"evil.example"}},
			},
		},
		"messages": []map[string]any{
			{"error_message": `[id "942100"] [msg "SQLi"] [data "` + dirtyData + `"] [severity "critical"] [tag "attack-sqli"]`},
		},
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	ev, ok := ParseRecord(line)
	if !ok {
		t.Fatal("should parse")
	}
	if strings.ContainsAny(ev.MatchedData, string([]byte{0, 27})) {
		t.Fatalf("control chars not stripped from matched data: %q", ev.MatchedData)
	}
	if !strings.Contains(ev.MatchedData, "PWN") {
		t.Fatalf("expected sanitized payload text retained: %q", ev.MatchedData)
	}
	if ev.Interrupted {
		t.Fatal("a detection-only record must not be marked interrupted")
	}
}

// With only the generic blocking rule (no specific attack rule), that rule is
// used rather than dropping the event.
func TestParseAnomalyOnlyPicksBlockingRule(t *testing.T) {
	rec := `{"transaction":{"unix_timestamp":1,"client_ip":"1.1.1.1","request":{"method":"GET","uri":"/","headers":{"host":["h"]}},"is_interrupted":true},"messages":[{"error_message":"[id \"949110\"] [msg \"Inbound Anomaly Score Exceeded\"] [severity \"unknown\"] [tag \"anomaly-evaluation\"]"}]}`
	ev, ok := ParseRecord([]byte(rec))
	if !ok || ev.RuleID != 949110 {
		t.Fatalf("should pick the blocking rule 949110: ok=%v id=%d", ok, ev.RuleID)
	}
}

func TestParseHostFallsBackToServerID(t *testing.T) {
	rec := `{"transaction":{"unix_timestamp":1,"client_ip":"1.1.1.1","server_id":"fallback.example","request":{"method":"GET","uri":"/","headers":{}},"is_interrupted":true},"messages":[{"error_message":"[id \"942100\"] [tag \"attack-sqli\"] [severity \"critical\"]"}]}`
	ev, ok := ParseRecord([]byte(rec))
	if !ok || ev.Host != "fallback.example" {
		t.Fatalf("host should fall back to server_id: ok=%v host=%q", ok, ev.Host)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	for _, bad := range []string{
		"", "   ", "not json at all",
		`{"transaction":{}}`,               // no messages
		`{"transaction":{},"messages":[]}`, // empty messages
	} {
		if _, ok := ParseRecord([]byte(bad)); ok {
			t.Errorf("should reject: %q", bad)
		}
	}
}
