package wafconfig

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// MaxBlockPageBytes bounds the operator's refusal page. It is served on every
	// refusal, so an unbounded one would turn a flood of blocks into a bandwidth
	// problem of its own.
	MaxBlockPageBytes = 64 << 10
	// DefaultRetentionDays is how long attack records are kept when the operator
	// has not said otherwise.
	DefaultRetentionDays = 30
	// MaxRetentionDays bounds retention so a typo cannot mean "keep forever".
	MaxRetentionDays = 3650
)

// BlockPage is the operator's custom refusal response.
type BlockPage struct {
	// Status is the HTTP status served on a refusal. Zero means 403.
	Status int `json:"status,omitempty"`
	// HTML is the page body. Empty keeps the built-in page. The two placeholders
	// {{ip}} and {{time}} are substituted by the data plane, HTML-escaped.
	HTML string `json:"html,omitempty"`
}

// IsZero reports the built-in refusal page, which is emitted as an absent object.
func (p *BlockPage) IsZero() bool {
	return p == nil || (p.Status == 0 && strings.TrimSpace(p.HTML) == "")
}

// allowedBlockStatuses mirrors the data plane's closed set. A 5xx would blame
// the origin for a decision the WAF made, and a 3xx would turn a refusal into a
// redirect the operator would then have to secure.
var allowedBlockStatuses = map[int]struct{}{200: {}, 403: {}, 404: {}}

// NormalizeBlockPage validates the refusal page.
func NormalizeBlockPage(p BlockPage) (BlockPage, error) {
	if len(p.HTML) > MaxBlockPageBytes {
		return BlockPage{}, fmt.Errorf("block page is %d bytes, limit is %d", len(p.HTML), MaxBlockPageBytes)
	}
	if strings.TrimSpace(p.HTML) == "" {
		p.HTML = ""
	}
	if p.Status != 0 {
		if _, ok := allowedBlockStatuses[p.Status]; !ok {
			return BlockPage{}, fmt.Errorf("block page status %d is not one of 200, 403 or 404", p.Status)
		}
	}
	return p, nil
}

// EventKinds is the closed set of enforcement record kinds, mirroring the data
// plane's vocabulary. It is exported so the panel can offer exactly these and no
// others.
var EventKinds = []string{
	"acl-deny",
	"custom-rule",
	"region",
	"ratelimit",
	"ban",
	"banned",
	"ban-released",
	"unknown-host",
	"oversize-body",
	"challenge",
}

// LogSettings is the panel-wide record policy.
//
// RetentionDays is a CONTROL-PLANE concern and is deliberately not sent to the
// data plane: the panel owns the database the records end up in, and letting the
// gateway delete them would put deletion on the side that has no idea what has
// already been ingested. ExcludedKinds IS sent, because not writing a record at
// all is something only the writer can do.
type LogSettings struct {
	RetentionDays int      `json:"retentionDays,omitempty"`
	ExcludedKinds []string `json:"excludedKinds,omitempty"`
}

// GatewayLogSettings is the subset the data plane is given.
type GatewayLogSettings struct {
	ExcludedKinds []string `json:"excludedKinds,omitempty"`
}

// IsZero reports the default record policy.
func (l *LogSettings) IsZero() bool {
	return l == nil || (l.RetentionDays == 0 && len(l.ExcludedKinds) == 0)
}

// NormalizeLogSettings validates and canonicalizes the record policy.
func NormalizeLogSettings(l LogSettings) (LogSettings, error) {
	if l.RetentionDays < 0 || l.RetentionDays > MaxRetentionDays {
		return LogSettings{}, fmt.Errorf("retention must be between 1 and %d days", MaxRetentionDays)
	}
	known := make(map[string]struct{}, len(EventKinds))
	for _, k := range EventKinds {
		known[k] = struct{}{}
	}
	seen := make(map[string]struct{}, len(l.ExcludedKinds))
	out := make([]string, 0, len(l.ExcludedKinds))
	for _, k := range l.ExcludedKinds {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		// A misspelled kind is refused rather than ignored: silently accepting it
		// would leave the operator believing they had switched something off.
		if _, ok := known[k]; !ok {
			return LogSettings{}, fmt.Errorf("unknown record kind %q", k)
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) == 0 {
		out = nil
	}
	l.ExcludedKinds = out
	return l, nil
}

// RetentionDaysOr resolves the effective retention.
func (l *LogSettings) RetentionDaysOr(fallback int) int {
	if l != nil && l.RetentionDays > 0 {
		return l.RetentionDays
	}
	if fallback > 0 {
		return fallback
	}
	return DefaultRetentionDays
}
