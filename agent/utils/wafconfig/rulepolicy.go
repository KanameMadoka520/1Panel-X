package wafconfig

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// MaxAllowedMethods bounds the HTTP method allow-list.
const MaxAllowedMethods = 64

// methodPattern matches a syntactically valid HTTP method token.
//
// This is a security control, not tidiness: the gateway interpolates the list
// into a SecAction directive, so a token carrying a quote or a newline could
// append arbitrary engine directives — and "SecRuleEngine Off" would silently
// disable the WAF for every site sharing that compiled engine. The gateway
// validates again on load; this rejects it while the operator is still looking
// at the form.
var methodPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,19}$`)

// RulePolicy is a site's detection policy.
//
// Every field is expressed so its ZERO VALUE is the safe one: a detection class
// is only switched off by an explicit "disable", never by omission, so a policy
// that fails to round-trip cannot silently drop protection.
type RulePolicy struct {
	DisableSQLi bool `json:"disableSqli,omitempty"`
	DisableXSS  bool `json:"disableXss,omitempty"`
	// Strict raises the rule set's paranoia level: stricter checks at the cost of
	// more false positives.
	Strict bool `json:"strict,omitempty"`
	// AllowedMethods is the HTTP method allow-list. Empty leaves the rule set's
	// own default in force rather than allowing everything.
	AllowedMethods []string `json:"allowedMethods,omitempty"`
	// BannedUploadExts are extensions refused when they name an uploaded file.
	// Empty applies no extension check of our own; the rule set's built-in upload
	// rules are unaffected either way.
	BannedUploadExts []string `json:"bannedUploadExts,omitempty"`
}

// IsZero reports the fully-protecting default policy, which is emitted as an
// absent object so the config stays small and older gateways keep working.
func (p RulePolicy) IsZero() bool {
	return !p.DisableSQLi && !p.DisableXSS && !p.Strict &&
		len(p.AllowedMethods) == 0 && len(p.BannedUploadExts) == 0
}

// NormalizeRulePolicy validates and canonicalizes a policy. Canonical ordering
// matters twice over: it keeps the config-generation digest stable for an
// unchanged policy, and it lets two sites with equivalent policies share one
// compiled engine instead of paying for a second full rule set.
func NormalizeRulePolicy(p RulePolicy) (RulePolicy, error) {
	methods, err := NormalizeMethods(p.AllowedMethods)
	if err != nil {
		return RulePolicy{}, err
	}
	p.AllowedMethods = methods
	exts, err := NormalizeUploadExtensions(p.BannedUploadExts)
	if err != nil {
		return RulePolicy{}, err
	}
	p.BannedUploadExts = exts
	return p, nil
}

// MaxBannedUploadExtensions bounds the banned upload extension list.
const MaxBannedUploadExtensions = 64

// uploadExtensionPattern is a security control for the same reason
// methodPattern is: the gateway interpolates each extension into a SecRule
// regular expression inside a quoted directive, so a value carrying a quote, a
// newline or a regex metacharacter could terminate that directive and append
// another one. Restricting the charset to alphanumerics means no escaping is
// needed and none can be forgotten. The gateway validates again on load; this
// rejects it while the operator is still looking at the form.
var uploadExtensionPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,15}$`)

// NormalizeUploadExtensions lower-cases, strips a leading dot, de-duplicates and
// sorts the banned upload extension list.
func NormalizeUploadExtensions(exts []string) ([]string, error) {
	if len(exts) == 0 {
		return nil, nil
	}
	if len(exts) > MaxBannedUploadExtensions {
		return nil, fmt.Errorf("too many banned upload extensions (%d), limit is %d", len(exts), MaxBannedUploadExtensions)
	}
	seen := make(map[string]struct{}, len(exts))
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.TrimPrefix(strings.TrimSpace(e), ".")
		if e == "" {
			continue
		}
		if !uploadExtensionPattern.MatchString(e) {
			return nil, fmt.Errorf("invalid upload extension %q (letters and digits only)", e)
		}
		e = strings.ToLower(e)
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

// NormalizeMethods upper-cases, de-duplicates and sorts the method allow-list.
func NormalizeMethods(methods []string) ([]string, error) {
	if len(methods) == 0 {
		return nil, nil
	}
	if len(methods) > MaxAllowedMethods {
		return nil, fmt.Errorf("too many allowed methods (%d), limit is %d", len(methods), MaxAllowedMethods)
	}
	seen := make(map[string]struct{}, len(methods))
	out := make([]string, 0, len(methods))
	for _, m := range methods {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if !methodPattern.MatchString(m) {
			return nil, fmt.Errorf("invalid HTTP method %q", m)
		}
		m = strings.ToUpper(m)
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

// MergeRulePolicy overlays a site's policy on the panel-wide default.
//
// A site that has stored no policy of its own inherits the panel default
// wholesale; once it stores one, that policy is authoritative for every field.
// Field-by-field merging is deliberately NOT done: a boolean cannot express
// "not set", so merging per field would make it impossible for a site to switch
// something back ON once the panel default switched it off.
func MergeRulePolicy(global *RulePolicy, site *RulePolicy) *RulePolicy {
	if site != nil {
		return site
	}
	return global
}
