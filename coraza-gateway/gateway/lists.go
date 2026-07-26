package gateway

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// ListTarget names the request attribute a list entry is compared against.
type ListTarget string

const (
	ListTargetIP        ListTarget = "ip"
	ListTargetURL       ListTarget = "url"
	ListTargetUserAgent ListTarget = "ua"
	// ListTargetIPGroup matches the client address against a named group of
	// addresses/CIDRs, so one shared set can be referenced from several entries.
	ListTargetIPGroup ListTarget = "ipgroup"
)

// ListMatch is how a pattern is compared.
type ListMatch string

const (
	ListMatchExact    ListMatch = "exact"
	ListMatchPrefix   ListMatch = "prefix"
	ListMatchSuffix   ListMatch = "suffix"
	ListMatchContains ListMatch = "contains"
	ListMatchRegex    ListMatch = "regex"
)

// List names which of the two lists an entry belongs to.
type List string

const (
	ListDeny  List = "deny"
	ListAllow List = "allow"
)

const (
	maxListPatternBytes = 512
	maxListEntries      = 2000
	maxIPGroupEntries   = 4096
)

// ListRule is one black/white list entry.
//
// Entries are panel-wide: the upstream product presents its lists as a single
// top-level set with no site selector, and this mirrors that. Per-site IP lists
// remain available as an additional, narrower control.
type ListRule struct {
	List    List       `json:"list"`
	Target  ListTarget `json:"target"`
	Match   ListMatch  `json:"match,omitempty"`
	Pattern string     `json:"pattern"`
	// Remark is operator-facing only; it never affects matching.
	Remark string `json:"remark,omitempty"`
}

// IPGroup is a named set of addresses/CIDRs referenced by ipgroup rules.
type IPGroup struct {
	Name    string   `json:"name"`
	Entries []string `json:"entries"`
}

// compiledListRule is a validated, ready-to-evaluate entry.
type compiledListRule struct {
	rule    ListRule
	nets    []*net.IPNet   // ip / ipgroup targets
	re      *regexp.Regexp // regex match
	pattern string         // non-regex match, lower-cased for ua/url comparison
	// label identifies the rule in enforcement records.
	label string
}

// listMatcher evaluates the panel-wide lists for one request.
//
// Precedence is deliberate and fixed: an explicit DENY outranks an explicit
// ALLOW, and both outrank every automatic mechanism (bans, rate limits, CRS).
// An allow entry is an exemption from automatic machinery, not a licence that
// overrides another operator's explicit refusal.
type listMatcher struct {
	deny  []compiledListRule
	allow []compiledListRule
}

func (m *listMatcher) empty() bool {
	return m == nil || (len(m.deny) == 0 && len(m.allow) == 0)
}

// newListMatcher validates and compiles the panel-wide lists. Any invalid entry
// is a hard error: the config is refused as a whole rather than silently
// enforcing a subset of what the operator configured.
func newListMatcher(rules []ListRule, groups []IPGroup) (*listMatcher, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	if len(rules) > maxListEntries {
		return nil, fmt.Errorf("too many list entries (%d), limit is %d", len(rules), maxListEntries)
	}
	byName := make(map[string][]*net.IPNet, len(groups))
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			return nil, fmt.Errorf("ip group has an empty name")
		}
		if _, dup := byName[name]; dup {
			return nil, fmt.Errorf("duplicate ip group %q", name)
		}
		if len(g.Entries) > maxIPGroupEntries {
			return nil, fmt.Errorf("ip group %q has %d entries, limit is %d", name, len(g.Entries), maxIPGroupEntries)
		}
		nets, err := parseIPNets(g.Entries)
		if err != nil {
			return nil, fmt.Errorf("ip group %q %w", name, err)
		}
		byName[name] = nets
	}

	m := &listMatcher{}
	for i, r := range rules {
		compiled, err := compileListRule(r, byName)
		if err != nil {
			return nil, fmt.Errorf("list entry %d: %w", i, err)
		}
		if r.List == ListAllow {
			m.allow = append(m.allow, compiled)
			continue
		}
		m.deny = append(m.deny, compiled)
	}
	if m.empty() {
		return nil, nil
	}
	return m, nil
}

func compileListRule(r ListRule, groups map[string][]*net.IPNet) (compiledListRule, error) {
	switch r.List {
	case ListDeny, ListAllow:
	case "":
		r.List = ListDeny
	default:
		return compiledListRule{}, fmt.Errorf("unknown list %q", r.List)
	}
	pattern := strings.TrimSpace(r.Pattern)
	if pattern == "" {
		return compiledListRule{}, fmt.Errorf("empty pattern")
	}
	if len(pattern) > maxListPatternBytes {
		return compiledListRule{}, fmt.Errorf("pattern is %d bytes, limit is %d", len(pattern), maxListPatternBytes)
	}
	out := compiledListRule{rule: r, label: string(r.List) + ":" + string(r.Target)}

	switch r.Target {
	case ListTargetIP:
		nets, err := parseIPNets([]string{pattern})
		if err != nil {
			return compiledListRule{}, err
		}
		out.nets = nets
	case ListTargetIPGroup:
		nets, ok := groups[pattern]
		if !ok {
			return compiledListRule{}, fmt.Errorf("references unknown ip group %q", pattern)
		}
		out.nets = nets
		out.label += ":" + pattern
	case ListTargetURL, ListTargetUserAgent:
		switch r.Match {
		case ListMatchRegex:
			// Go's regexp is RE2: matching is linear in the input, so a hostile
			// pattern cannot cause catastrophic backtracking. Compiling here also
			// means a malformed pattern fails the config load, not a request.
			re, err := regexp.Compile(pattern)
			if err != nil {
				return compiledListRule{}, fmt.Errorf("invalid regular expression: %w", err)
			}
			out.re = re
		case ListMatchExact, ListMatchPrefix, ListMatchSuffix, ListMatchContains:
			out.pattern = strings.ToLower(pattern)
		case "":
			out.rule.Match = ListMatchContains
			out.pattern = strings.ToLower(pattern)
		default:
			return compiledListRule{}, fmt.Errorf("unknown match type %q", r.Match)
		}
	default:
		return compiledListRule{}, fmt.Errorf("unknown target %q", r.Target)
	}
	return out, nil
}

// matches reports whether this entry matches the request.
func (c compiledListRule) matches(r *http.Request, ip net.IP) bool {
	switch c.rule.Target {
	case ListTargetIP, ListTargetIPGroup:
		return ip != nil && matchAnyNet(c.nets, ip)
	case ListTargetURL:
		return c.matchText(r.URL.RequestURI())
	case ListTargetUserAgent:
		return c.matchText(r.Header.Get("User-Agent"))
	}
	return false
}

func (c compiledListRule) matchText(value string) bool {
	if value == "" {
		return false
	}
	if c.re != nil {
		return c.re.MatchString(value)
	}
	lowered := strings.ToLower(value)
	switch c.rule.Match {
	case ListMatchExact:
		return lowered == c.pattern
	case ListMatchPrefix:
		return strings.HasPrefix(lowered, c.pattern)
	case ListMatchSuffix:
		return strings.HasSuffix(lowered, c.pattern)
	default:
		return strings.Contains(lowered, c.pattern)
	}
}

// decide evaluates the lists, returning the decision and the label of the entry
// that produced it.
func (m *listMatcher) decide(r *http.Request, ip net.IP) (aclDecision, string) {
	if m == nil {
		return aclNormal, ""
	}
	for _, c := range m.deny {
		if c.matches(r, ip) {
			return aclDeny, c.label
		}
	}
	for _, c := range m.allow {
		if c.matches(r, ip) {
			return aclAllow, c.label
		}
	}
	return aclNormal, ""
}
