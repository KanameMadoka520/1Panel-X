package gateway

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// Custom rules are evaluated in Go, NOT compiled into engine directives.
//
// The engine takes its configuration as a raw directive string, so building
// rules out of operator-supplied conditions would mean interpolating arbitrary
// text into that string. One newline in one field would be enough to append
// `SecRuleEngine Off`, silently disabling the WAF for every site sharing that
// compiled engine. Evaluating here keeps operator input as data and never as
// configuration syntax — and it means a rule change costs nothing to apply,
// where a directive change would force a full rule-set recompile.

// CustomField names the request attribute a condition inspects.
type CustomField string

const (
	FieldIP        CustomField = "ip"
	FieldHost      CustomField = "host"
	FieldMethod    CustomField = "method"
	FieldURI       CustomField = "uri"
	FieldPath      CustomField = "path"
	FieldQuery     CustomField = "query"
	FieldUserAgent CustomField = "ua"
	FieldReferer   CustomField = "referer"
	FieldHeader    CustomField = "header"
	FieldCookie    CustomField = "cookie"
)

// CustomAction is what a matching rule does.
type CustomAction string

const (
	// CustomDeny refuses the request outright with the generic block page.
	CustomDeny CustomAction = "deny"
	// CustomAllow proxies the request without rule-set inspection. It is an
	// operator exemption, so it also outranks bans and frequency limits.
	CustomAllow CustomAction = "allow"
	// CustomLog records the match and lets evaluation continue. It exists so an
	// operator can watch a candidate rule before arming it.
	CustomLog CustomAction = "log"
)

const (
	maxCustomRules         = 200
	maxCustomConditions    = 8
	maxCustomPatternBytes  = 512
	maxCustomRuleNameBytes = 64
)

// customNamePattern bounds a header or cookie name. Header names are tokens by
// definition, so anything outside this set is a typo or an attempt at something
// else.
var customNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// CustomCondition is one test against one request attribute. A rule's conditions
// are ANDed: every one must hold.
type CustomCondition struct {
	Field CustomField `json:"field"`
	// Name selects which header or cookie to read. Required for those two fields
	// and rejected for every other, so a condition can never silently inspect
	// something other than what it names.
	Name  string    `json:"name,omitempty"`
	Match ListMatch `json:"match,omitempty"`
	// Pattern is compared per Match. For the ip field it is an address or CIDR.
	Pattern string `json:"pattern"`
	// Negate inverts this condition alone, not the rule.
	Negate bool `json:"negate,omitempty"`
}

// CustomRule is one operator-authored rule.
type CustomRule struct {
	// Name is operator-facing and appears in enforcement records, so a decision
	// can be traced back to the rule that made it.
	Name       string            `json:"name,omitempty"`
	Action     CustomAction      `json:"action"`
	Conditions []CustomCondition `json:"conditions"`
}

type compiledCondition struct {
	cond    CustomCondition
	nets    []*net.IPNet
	re      *regexp.Regexp
	pattern string
}

type compiledCustomRule struct {
	action     CustomAction
	label      string
	conditions []compiledCondition
}

// customMatcher evaluates the ordered rule list. The FIRST rule whose conditions
// all hold decides the outcome; a `log` rule records and lets the scan continue,
// so an armed rule further down still wins.
type customMatcher struct {
	rules []compiledCustomRule
}

func (m *customMatcher) empty() bool { return m == nil || len(m.rules) == 0 }

// newCustomMatcher validates and compiles the rules. Any invalid rule fails the
// whole config: enforcing a subset of what the operator wrote, without saying
// so, is worse than refusing the change.
func newCustomMatcher(rules []CustomRule) (*customMatcher, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	if len(rules) > maxCustomRules {
		return nil, fmt.Errorf("too many custom rules (%d), limit is %d", len(rules), maxCustomRules)
	}
	m := &customMatcher{rules: make([]compiledCustomRule, 0, len(rules))}
	for i, r := range rules {
		compiled, err := compileCustomRule(r, i)
		if err != nil {
			return nil, fmt.Errorf("custom rule %d: %w", i, err)
		}
		m.rules = append(m.rules, compiled)
	}
	return m, nil
}

func compileCustomRule(r CustomRule, index int) (compiledCustomRule, error) {
	switch r.Action {
	case CustomDeny, CustomAllow, CustomLog:
	case "":
		r.Action = CustomDeny
	default:
		return compiledCustomRule{}, fmt.Errorf("unknown action %q", r.Action)
	}
	// A rule with no conditions holds for every request. Combined with `deny`
	// that is a self-inflicted outage of every protected site, and combined with
	// `allow` it disables the WAF entirely — either way it is far more likely to
	// be an unfinished form than an intent.
	if len(r.Conditions) == 0 {
		return compiledCustomRule{}, fmt.Errorf("needs at least one condition")
	}
	if len(r.Conditions) > maxCustomConditions {
		return compiledCustomRule{}, fmt.Errorf("has %d conditions, limit is %d", len(r.Conditions), maxCustomConditions)
	}
	name := strings.TrimSpace(r.Name)
	if len(name) > maxCustomRuleNameBytes {
		name = name[:maxCustomRuleNameBytes]
	}
	label := "custom:" + string(r.Action)
	if name != "" {
		label += ":" + name
	} else {
		label += fmt.Sprintf(":#%d", index)
	}
	out := compiledCustomRule{action: r.Action, label: label}
	for i, c := range r.Conditions {
		compiled, err := compileCustomCondition(c)
		if err != nil {
			return compiledCustomRule{}, fmt.Errorf("condition %d: %w", i, err)
		}
		out.conditions = append(out.conditions, compiled)
	}
	return out, nil
}

func compileCustomCondition(c CustomCondition) (compiledCondition, error) {
	pattern := strings.TrimSpace(c.Pattern)
	if pattern == "" {
		return compiledCondition{}, fmt.Errorf("empty pattern")
	}
	if len(pattern) > maxCustomPatternBytes {
		return compiledCondition{}, fmt.Errorf("pattern is %d bytes, limit is %d", len(pattern), maxCustomPatternBytes)
	}
	name := strings.TrimSpace(c.Name)
	needsName := c.Field == FieldHeader || c.Field == FieldCookie
	if needsName {
		if !customNamePattern.MatchString(name) {
			return compiledCondition{}, fmt.Errorf("field %q needs a valid header/cookie name, got %q", c.Field, c.Name)
		}
	} else if name != "" {
		return compiledCondition{}, fmt.Errorf("field %q does not take a name", c.Field)
	}
	c.Name = name

	out := compiledCondition{cond: c}
	switch c.Field {
	case FieldIP:
		// An address condition is always a network membership test, so a match
		// type here would only be a way to get it subtly wrong.
		if c.Match != "" {
			return compiledCondition{}, fmt.Errorf("field %q does not take a match type", c.Field)
		}
		nets, err := parseIPNets([]string{pattern})
		if err != nil {
			return compiledCondition{}, err
		}
		out.nets = nets
	case FieldHost, FieldMethod, FieldURI, FieldPath, FieldQuery, FieldUserAgent, FieldReferer, FieldHeader, FieldCookie:
		switch c.Match {
		case ListMatchRegex:
			// Go's regexp is RE2: matching is linear in the input, so a hostile
			// pattern cannot cause catastrophic backtracking, and a malformed one
			// fails the config load rather than a request.
			re, err := regexp.Compile(pattern)
			if err != nil {
				return compiledCondition{}, fmt.Errorf("invalid regular expression: %w", err)
			}
			out.re = re
		case ListMatchExact, ListMatchPrefix, ListMatchSuffix, ListMatchContains:
			out.pattern = strings.ToLower(pattern)
		case "":
			out.cond.Match = ListMatchContains
			out.pattern = strings.ToLower(pattern)
		default:
			return compiledCondition{}, fmt.Errorf("unknown match type %q", c.Match)
		}
	default:
		return compiledCondition{}, fmt.Errorf("unknown field %q", c.Field)
	}
	return out, nil
}

// value extracts the request attribute this condition inspects.
func (c compiledCondition) value(r *http.Request) string {
	switch c.cond.Field {
	case FieldHost:
		return r.Host
	case FieldMethod:
		return r.Method
	case FieldURI:
		return r.URL.RequestURI()
	case FieldPath:
		return r.URL.Path
	case FieldQuery:
		return r.URL.RawQuery
	case FieldUserAgent:
		return r.Header.Get("User-Agent")
	case FieldReferer:
		return r.Header.Get("Referer")
	case FieldHeader:
		return r.Header.Get(c.cond.Name)
	case FieldCookie:
		if ck, err := r.Cookie(c.cond.Name); err == nil {
			return ck.Value
		}
		return ""
	}
	return ""
}

func (c compiledCondition) holds(r *http.Request, ip net.IP) bool {
	matched := c.rawMatch(r, ip)
	if c.cond.Negate {
		return !matched
	}
	return matched
}

func (c compiledCondition) rawMatch(r *http.Request, ip net.IP) bool {
	if c.cond.Field == FieldIP {
		return ip != nil && matchAnyNet(c.nets, ip)
	}
	value := c.value(r)
	if c.re != nil {
		// An absent header is an empty string, and an empty string can legitimately
		// match a regular expression such as `^$`, so no early return here.
		return c.re.MatchString(value)
	}
	if value == "" {
		return false
	}
	lowered := strings.ToLower(value)
	switch c.cond.Match {
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

func (rule compiledCustomRule) matches(r *http.Request, ip net.IP) bool {
	for _, c := range rule.conditions {
		if !c.holds(r, ip) {
			return false
		}
	}
	return true
}

// customOutcome is the result of evaluating the rule list.
type customOutcome struct {
	Decision aclDecision
	// Rule labels whichever rule produced Decision.
	Rule string
	// Observed labels the first `log` rule that matched. It is reported
	// independently of Decision so a watched rule leaves a record even when the
	// request goes on to be allowed normally.
	Observed string
}

func (m *customMatcher) decide(r *http.Request, ip net.IP) customOutcome {
	out := customOutcome{Decision: aclNormal}
	if m == nil {
		return out
	}
	for _, rule := range m.rules {
		if !rule.matches(r, ip) {
			continue
		}
		switch rule.action {
		case CustomDeny:
			out.Decision, out.Rule = aclDeny, rule.label
			return out
		case CustomAllow:
			out.Decision, out.Rule = aclAllow, rule.label
			return out
		case CustomLog:
			if out.Observed == "" {
				out.Observed = rule.label
			}
		}
	}
	return out
}
