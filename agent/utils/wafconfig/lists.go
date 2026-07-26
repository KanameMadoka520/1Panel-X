package wafconfig

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Black/white list vocabulary. These mirror the data plane's contract exactly;
// the gateway validates them again on load, so a mismatch fails closed instead
// of being silently dropped.
const (
	ListDeny  = "deny"
	ListAllow = "allow"

	ListTargetIP        = "ip"
	ListTargetIPGroup   = "ipgroup"
	ListTargetURL       = "url"
	ListTargetUserAgent = "ua"

	ListMatchExact    = "exact"
	ListMatchPrefix   = "prefix"
	ListMatchContains = "contains"
	ListMatchRegex    = "regex"
)

const (
	MaxListPatternBytes = 512
	MaxListEntries      = 2000
	MaxIPGroupEntries   = 4096
	MaxIPGroups         = 128
)

// ListRule is one panel-wide black/white list entry.
type ListRule struct {
	List    string `json:"list"`
	Target  string `json:"target"`
	Match   string `json:"match,omitempty"`
	Pattern string `json:"pattern"`
	Remark  string `json:"remark,omitempty"`
}

// IPGroup is a named address set referenced by ipgroup entries.
type IPGroup struct {
	Name    string   `json:"name"`
	Entries []string `json:"entries"`
}

// NormalizeIPGroups validates and canonicalizes the named address sets.
func NormalizeIPGroups(groups []IPGroup) ([]IPGroup, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	if len(groups) > MaxIPGroups {
		return nil, fmt.Errorf("too many IP groups (%d), limit is %d", len(groups), MaxIPGroups)
	}
	seen := make(map[string]struct{}, len(groups))
	out := make([]IPGroup, 0, len(groups))
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			return nil, fmt.Errorf("IP group name cannot be empty")
		}
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("duplicate IP group %q", name)
		}
		seen[name] = struct{}{}
		if len(g.Entries) > MaxIPGroupEntries {
			return nil, fmt.Errorf("IP group %q has %d entries, limit is %d", name, len(g.Entries), MaxIPGroupEntries)
		}
		entries, err := NormalizeIPList(g.Entries)
		if err != nil {
			return nil, fmt.Errorf("IP group %q: %w", name, err)
		}
		out = append(out, IPGroup{Name: name, Entries: entries})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// NormalizeListRules validates the panel-wide lists against the available IP
// groups and returns them in a deterministic order.
//
// Ordering is canonical so an unchanged policy always yields the same config
// generation digest. Ordering across the two lists carries no meaning: the
// gateway evaluates every deny entry before any allow entry regardless.
func NormalizeListRules(rules []ListRule, groups []IPGroup) ([]ListRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	if len(rules) > MaxListEntries {
		return nil, fmt.Errorf("too many list entries (%d), limit is %d", len(rules), MaxListEntries)
	}
	known := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		known[strings.TrimSpace(g.Name)] = struct{}{}
	}

	seen := make(map[string]struct{}, len(rules))
	out := make([]ListRule, 0, len(rules))
	for _, r := range rules {
		r.List = strings.TrimSpace(r.List)
		r.Target = strings.TrimSpace(r.Target)
		r.Match = strings.TrimSpace(r.Match)
		r.Pattern = strings.TrimSpace(r.Pattern)
		r.Remark = strings.TrimSpace(r.Remark)

		switch r.List {
		case ListDeny, ListAllow:
		case "":
			r.List = ListDeny
		default:
			return nil, fmt.Errorf("unknown list %q", r.List)
		}
		if r.Pattern == "" {
			return nil, fmt.Errorf("list entry pattern cannot be empty")
		}
		if len(r.Pattern) > MaxListPatternBytes {
			return nil, fmt.Errorf("list pattern is %d bytes, limit is %d", len(r.Pattern), MaxListPatternBytes)
		}

		switch r.Target {
		case ListTargetIP:
			canonical, err := NormalizeIPList([]string{r.Pattern})
			if err != nil {
				return nil, err
			}
			r.Pattern = canonical[0]
			// A match type is meaningless for an address comparison; clearing it
			// keeps the emitted config from carrying a flag nothing reads.
			r.Match = ""
		case ListTargetIPGroup:
			if _, ok := known[r.Pattern]; !ok {
				return nil, fmt.Errorf("list entry references unknown IP group %q", r.Pattern)
			}
			r.Match = ""
		case ListTargetURL, ListTargetUserAgent:
			switch r.Match {
			case ListMatchExact, ListMatchPrefix, ListMatchContains:
			case "":
				r.Match = ListMatchContains
			case ListMatchRegex:
				// Compiled here so a bad expression is rejected while the operator
				// is still looking at the form, not when a request arrives.
				if _, err := regexp.Compile(r.Pattern); err != nil {
					return nil, fmt.Errorf("invalid regular expression %q: %w", r.Pattern, err)
				}
			default:
				return nil, fmt.Errorf("unknown match type %q", r.Match)
			}
		default:
			return nil, fmt.Errorf("unknown list target %q", r.Target)
		}

		key := r.List + "|" + r.Target + "|" + r.Match + "|" + r.Pattern
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.List != b.List {
			return a.List < b.List
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.Match != b.Match {
			return a.Match < b.Match
		}
		return a.Pattern < b.Pattern
	})
	return out, nil
}
