package wafconfig

import (
	"fmt"
	"regexp"
	"strings"
)

// Custom-rule vocabulary. These mirror the data plane's contract exactly; the
// gateway validates them again on load, so a mismatch fails closed instead of
// being silently dropped.
const (
	CustomActionDeny  = "deny"
	CustomActionAllow = "allow"
	CustomActionLog   = "log"

	CustomFieldIP     = "ip"
	CustomFieldHost   = "host"
	CustomFieldMethod = "method"
	// CustomFieldURL is what the panel calls "URL" and is the request PATH,
	// without the query string — the reading the upstream product's own examples
	// require. CustomFieldPath is the same thing under its older name, still
	// accepted so a rule stored before the rename keeps evaluating.
	CustomFieldURL       = "url"
	CustomFieldPath      = "path"
	CustomFieldURI       = "uri"
	CustomFieldQuery     = "query"
	CustomFieldUserAgent = "ua"
	CustomFieldReferer   = "referer"
	CustomFieldHeader    = "header"
	CustomFieldCookie    = "cookie"

	// ListMatchSuffix is offered to custom rules and to the lists alike.
	ListMatchSuffix = "suffix"
)

const (
	MaxCustomRules        = 200
	MaxCustomConditions   = 8
	MaxCustomPatternBytes = 512
	MaxCustomRuleName     = 64
)

var customNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// CustomCondition is one test against one request attribute.
type CustomCondition struct {
	Field   string `json:"field"`
	Name    string `json:"name,omitempty"`
	Match   string `json:"match,omitempty"`
	Pattern string `json:"pattern"`
	Negate  bool   `json:"negate,omitempty"`
}

// CustomRule is one operator-authored rule. Conditions are ANDed.
type CustomRule struct {
	Name       string            `json:"name,omitempty"`
	Action     string            `json:"action"`
	Conditions []CustomCondition `json:"conditions"`
}

// NormalizeCustomRules validates and canonicalizes the rules.
//
// Order is PRESERVED, unlike the black/white lists: the data plane resolves the
// first matching rule, so reordering them would change which rule decides a
// request. That means the emitted order is part of the operator's policy and
// cannot be sorted for digest stability.
func NormalizeCustomRules(rules []CustomRule) ([]CustomRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	if len(rules) > MaxCustomRules {
		return nil, fmt.Errorf("too many custom rules (%d), limit is %d", len(rules), MaxCustomRules)
	}
	out := make([]CustomRule, 0, len(rules))
	for i, r := range rules {
		normalized, err := normalizeCustomRule(r)
		if err != nil {
			return nil, fmt.Errorf("custom rule %d: %w", i+1, err)
		}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeCustomRule(r CustomRule) (CustomRule, error) {
	r.Name = strings.TrimSpace(r.Name)
	if len(r.Name) > MaxCustomRuleName {
		return CustomRule{}, fmt.Errorf("name is longer than %d characters", MaxCustomRuleName)
	}
	r.Action = strings.TrimSpace(r.Action)
	switch r.Action {
	case CustomActionDeny, CustomActionAllow, CustomActionLog:
	case "":
		r.Action = CustomActionDeny
	default:
		return CustomRule{}, fmt.Errorf("unknown action %q", r.Action)
	}
	// A rule with no conditions holds for every request: with "deny" that is an
	// outage of every protected site, with "allow" it disables the WAF outright.
	if len(r.Conditions) == 0 {
		return CustomRule{}, fmt.Errorf("needs at least one condition")
	}
	if len(r.Conditions) > MaxCustomConditions {
		return CustomRule{}, fmt.Errorf("has %d conditions, limit is %d", len(r.Conditions), MaxCustomConditions)
	}
	conditions := make([]CustomCondition, 0, len(r.Conditions))
	for i, c := range r.Conditions {
		normalized, err := normalizeCustomCondition(c)
		if err != nil {
			return CustomRule{}, fmt.Errorf("condition %d: %w", i+1, err)
		}
		conditions = append(conditions, normalized)
	}
	r.Conditions = conditions
	return r, nil
}

func normalizeCustomCondition(c CustomCondition) (CustomCondition, error) {
	c.Field = strings.TrimSpace(c.Field)
	c.Match = strings.TrimSpace(c.Match)
	c.Name = strings.TrimSpace(c.Name)
	c.Pattern = strings.TrimSpace(c.Pattern)

	if c.Pattern == "" {
		return CustomCondition{}, fmt.Errorf("pattern cannot be empty")
	}
	if len(c.Pattern) > MaxCustomPatternBytes {
		return CustomCondition{}, fmt.Errorf("pattern is %d bytes, limit is %d", len(c.Pattern), MaxCustomPatternBytes)
	}
	needsName := c.Field == CustomFieldHeader || c.Field == CustomFieldCookie
	if needsName {
		if !customNamePattern.MatchString(c.Name) {
			return CustomCondition{}, fmt.Errorf("field %q needs a valid header/cookie name", c.Field)
		}
	} else if c.Name != "" {
		return CustomCondition{}, fmt.Errorf("field %q does not take a name", c.Field)
	}

	switch c.Field {
	case CustomFieldIP:
		if c.Match != "" {
			return CustomCondition{}, fmt.Errorf("field %q does not take a match type", c.Field)
		}
		canonical, err := NormalizeIPList([]string{c.Pattern})
		if err != nil {
			return CustomCondition{}, err
		}
		c.Pattern = canonical[0]
	case CustomFieldHost, CustomFieldMethod, CustomFieldURL, CustomFieldPath, CustomFieldURI,
		CustomFieldQuery, CustomFieldUserAgent, CustomFieldReferer, CustomFieldHeader, CustomFieldCookie:
		switch c.Match {
		case ListMatchExact, ListMatchPrefix, ListMatchSuffix, ListMatchContains:
		case "":
			c.Match = ListMatchContains
		case ListMatchRegex:
			// Compiled here so a bad expression is rejected while the operator is
			// still looking at the form, not when a request arrives.
			if _, err := regexp.Compile(c.Pattern); err != nil {
				return CustomCondition{}, fmt.Errorf("invalid regular expression %q: %w", c.Pattern, err)
			}
		default:
			return CustomCondition{}, fmt.Errorf("unknown match type %q", c.Match)
		}
	default:
		return CustomCondition{}, fmt.Errorf("unknown field %q", c.Field)
	}
	return c, nil
}
