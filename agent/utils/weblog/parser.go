// Package weblog is the pure, dependency-free core of 1Panel-X website access
// monitoring: it parses nginx access-log lines (the standard "main"/combined
// shape) into structured entries and aggregates them into time-series buckets +
// top-N rankings. Both halves are pure functions with no I/O, so they are fully
// unit-testable with fixture log lines (no live nginx). See
// .planning/research/WEBSITE-MONITORING-DESIGN.md (controls M1-M6).
package weblog

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxLineLen caps a log line before parsing (M2: ReDoS/DoS defense).
const maxLineLen = 16 * 1024

// maxFieldLen bounds a captured string field before it reaches the store/UI
// (M1: log-injection defense — a forged huge/hostile URI/UA/Referer is truncated).
const maxFieldLen = 2048

// AccessEntry is one parsed access-log line.
type AccessEntry struct {
	Time          time.Time
	IP            string
	Method        string
	URI           string
	Proto         string
	Status        int
	Bytes         int64
	Referer       string
	UserAgent     string
	XForwardedFor string
}

// nginx CLF local time, e.g. 13/Jul/2026:18:30:00 +0000
const clfTimeLayout = "02/Jan/2006:15:04:05 -0700"

// mainLineRe matches the nginx "main"/combined access-log shape:
//
//	$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent
//	    ["$http_referer" "$http_user_agent" ["$http_x_forwarded_for"]]
//
// The trailing quoted groups are optional so a bare "main" without referer/UA
// still parses. Go's regexp (RE2) runs in guaranteed linear time (no
// catastrophic backtracking), and the total input is bounded by maxLineLen +
// each captured field truncated by clean(), so unbounded char classes are safe.
var mainLineRe = regexp.MustCompile(
	`^(\S+) \S+ (?:\S+|-) \[([^\]]+)\] "([^"]*)" (\d{3}) (\d+|-)` +
		`(?: "([^"]*)" "([^"]*)")?(?: "([^"]*)")?`,
)

// ParseLine parses one access-log line. It returns ok=false (never panics) for
// a line it cannot understand, so a malformed/hostile line is skipped rather
// than corrupting aggregation.
func ParseLine(line string) (AccessEntry, bool) {
	if len(line) > maxLineLen {
		line = line[:maxLineLen]
	}
	line = strings.TrimRight(line, "\r\n")
	m := mainLineRe.FindStringSubmatch(line)
	if m == nil {
		return AccessEntry{}, false
	}
	ts, err := time.Parse(clfTimeLayout, m[2])
	if err != nil {
		return AccessEntry{}, false
	}
	status, err := strconv.Atoi(m[4])
	if err != nil {
		return AccessEntry{}, false
	}
	var bytes int64
	if m[5] != "-" {
		bytes, _ = strconv.ParseInt(m[5], 10, 64)
	}
	method, uri, proto := splitRequest(m[3])

	return AccessEntry{
		Time:          ts,
		IP:            clean(m[1]),
		Method:        clean(method),
		URI:           clean(uri),
		Proto:         clean(proto),
		Status:        status,
		Bytes:         bytes,
		Referer:       clean(m[6]),
		UserAgent:     clean(m[7]),
		XForwardedFor: clean(m[8]),
	}, true
}

// splitRequest breaks `GET /path?x=1 HTTP/1.1` into method / uri / proto. It is
// tolerant of a malformed request line (returns what it can).
func splitRequest(req string) (method, uri, proto string) {
	parts := strings.SplitN(req, " ", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], parts[1], ""
	case 1:
		return parts[0], "", ""
	default:
		return "", "", ""
	}
}

// clean bounds length and strips control characters (M1) so a forged field
// cannot inject newlines/escape sequences into the store or the UI.
func clean(s string) string {
	if s == "" || s == "-" {
		return ""
	}
	if len(s) > maxFieldLen {
		s = s[:maxFieldLen]
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' || (r >= 0x20 && r != 0x7f) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
