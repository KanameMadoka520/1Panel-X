// Package wafaudit parses the JSON audit records emitted by the coraza-gateway
// (one per rule-matching transaction) into structured, sanitized attack events
// for the WAF attack-event store (Phase 20). It is a pure, dependency-light
// parser: no I/O, fully unit-testable with fixture records. Every
// attacker-controlled field is run through the shared W6/M1 sanitizer
// (weblog.Clean) before it leaves this package.
package wafaudit

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/utils/weblog"
)

// Event is one parsed, sanitized WAF attack event (one blocked/flagged request).
type Event struct {
	TxID        string // Coraza transaction id — a stable per-request key for dedup
	Time        time.Time
	Host        string
	SourceIP    string
	Method      string
	URI         string
	RuleID      int
	RuleMsg     string
	Category    string // e.g. "sqli" (from CRS tag "attack-sqli")
	Severity    string
	MatchedData string
	HitCount    int  // number of rules that fired on this request
	Interrupted bool // true = the request was actually blocked (vs detection-only)
}

// corazaRecord is the subset of Coraza's JSON audit schema we consume.
type corazaRecord struct {
	Transaction struct {
		ID            string `json:"id"`
		UnixTimestamp int64  `json:"unix_timestamp"`
		ClientIP      string `json:"client_ip"`
		ServerID      string `json:"server_id"`
		IsInterrupted bool   `json:"is_interrupted"`
		Request       struct {
			Method  string              `json:"method"`
			URI     string              `json:"uri"`
			Headers map[string][]string `json:"headers"`
		} `json:"request"`
	} `json:"transaction"`
	Messages []struct {
		ErrorMessage string `json:"error_message"`
	} `json:"messages"`
}

// ParseRecord parses one JSON audit line. Returns ok=false (never panics) for a
// line it cannot understand or one carrying no rule messages, so a malformed or
// hostile record is skipped rather than corrupting the store.
func ParseRecord(line []byte) (Event, bool) {
	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 {
		return Event{}, false
	}
	var rec corazaRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return Event{}, false
	}
	if len(rec.Messages) == 0 {
		return Event{}, false
	}

	errMsgs := make([]string, len(rec.Messages))
	for i := range rec.Messages {
		errMsgs[i] = rec.Messages[i].ErrorMessage
	}
	primary := selectPrimary(errMsgs)
	return Event{
		TxID:        weblog.Clean(rec.Transaction.ID),
		Time:        time.Unix(0, rec.Transaction.UnixTimestamp).UTC(),
		Host:        weblog.Clean(hostOf(&rec)),
		SourceIP:    weblog.Clean(rec.Transaction.ClientIP),
		Method:      weblog.Clean(rec.Transaction.Request.Method),
		URI:         weblog.Clean(rec.Transaction.Request.URI),
		RuleID:      primary.id,
		RuleMsg:     weblog.Clean(primary.msg),
		Category:    weblog.Clean(primary.category),
		Severity:    weblog.Clean(primary.severity),
		MatchedData: weblog.Clean(primary.data),
		HitCount:    len(rec.Messages),
		Interrupted: rec.Transaction.IsInterrupted,
	}, true
}

func hostOf(rec *corazaRecord) string {
	for _, k := range []string{"host", "Host"} {
		if v, ok := rec.Transaction.Request.Headers[k]; ok && len(v) > 0 && v[0] != "" {
			return v[0]
		}
	}
	return rec.Transaction.ServerID
}

// Coraza puts the rich rule detail in the human-readable error_message as
// ModSecurity-style [key "value"] tokens, not in structured JSON fields, so we
// extract them here. RE2 (linear) with a non-greedy value keeps this safe on
// hostile input.
var bracketRe = regexp.MustCompile(`\[([a-z_]+) "(.*?)"\]`)

type parsedMsg struct {
	id       int
	msg      string
	severity string
	data     string
	category string
	attack   bool
}

func parseMessage(errMsg string) parsedMsg {
	var p parsedMsg
	for _, m := range bracketRe.FindAllStringSubmatch(errMsg, -1) {
		key, val := m[1], m[2]
		switch key {
		case "id":
			p.id, _ = strconv.Atoi(val)
		case "msg":
			p.msg = val
		case "severity":
			p.severity = val
		case "data":
			p.data = val
		case "tag":
			if !p.attack && strings.HasPrefix(val, "attack-") {
				p.category = strings.TrimPrefix(val, "attack-")
				p.attack = true
			}
		}
	}
	return p
}

// selectPrimary picks the most representative rule for the event: a specific
// attack rule (e.g. 942100 SQLi) is preferred over the generic anomaly-scoring
// blocking rule (949110), and higher severity wins.
func selectPrimary(errMsgs []string) parsedMsg {
	best := parsedMsg{}
	bestScore := -1
	for _, m := range errMsgs {
		p := parseMessage(m)
		score := severityScore(p.severity)
		if p.attack {
			score += 100
		}
		if score > bestScore {
			bestScore = score
			best = p
		}
	}
	return best
}

func severityScore(sev string) int {
	switch strings.ToLower(sev) {
	case "critical", "emergency", "alert":
		return 5
	case "error":
		return 4
	case "warning":
		return 3
	case "notice":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}
