package gateway

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The upload restriction is implemented as OUR OWN rule, not as a knob on the
// bundled rule set, because the rule set has no such knob:
//
//   - `tx.restricted_extensions` (901164 → 920440) matches the extension of the
//     REQUEST URL, not of an uploaded file. Setting it would block requests for
//     `/x.php` while happily accepting an upload named `shell.php`.
//   - Upload names are checked by 932180 via `@pmFromFile restricted-upload.data`,
//     and that data file lives inside the rule set's embedded, read-only
//     filesystem, so its contents cannot be replaced with the operator's list.
//
// The rule below therefore stands on its own: it inspects FILES (the client-
// supplied names of multipart parts) and denies directly rather than feeding the
// anomaly score, so one banned rule is one refusal.
const (
	maxUploadRules      = 64
	maxUploadRuleLength = 32
)

// uploadRulePattern bounds one rule.
//
// This is a security control, not tidiness. Each rule is interpolated into a
// SecRule regular expression inside a quoted directive, so a value carrying a
// quote, a newline or a regex metacharacter could terminate the directive and
// append another one — and `SecRuleEngine Off` would disable the WAF for every
// site sharing that compiled engine. The charset is restricted to the characters
// a file extension actually needs, and the one metacharacter it allows (`.`) is
// escaped when the pattern is built.
var uploadRulePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)

// normalizeUploadRules validates, lower-cases, de-duplicates and sorts the rule
// list. A leading dot is accepted and stripped, because that is how operators
// habitually write extensions.
func normalizeUploadRules(rules []string) ([]string, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	if len(rules) > maxUploadRules {
		return nil, fmt.Errorf("too many upload rules (%d), limit is %d", len(rules), maxUploadRules)
	}
	seen := make(map[string]struct{}, len(rules))
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		r = strings.TrimSpace(r)
		r = strings.TrimPrefix(r, ".")
		if r == "" {
			continue
		}
		if !uploadRulePattern.MatchString(r) {
			return nil, fmt.Errorf("invalid upload rule %q (letters, digits, dot, dash and underscore only, at most %d characters)", r, maxUploadRuleLength)
		}
		r = strings.ToLower(r)
		if _, dup := seen[r]; dup {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

// uploadRuleID is our own rule id. The rule set reserves 900000-999999 for
// itself, so we stay well clear of that whole space.
const uploadRuleID = 8000003

// uploadDenyDirective builds the SecRule for a canonical (already validated,
// space-separated) rule list. An empty list produces no rule at all, so a site
// that restricts nothing pays nothing.
func uploadDenyDirective(canonical string) string {
	if canonical == "" {
		return ""
	}
	parts := strings.Split(canonical, " ")
	for i, p := range parts {
		// `.` is the only metacharacter the charset admits; escaping it keeps a
		// rule like `.php` from also matching `xphp`.
		parts[i] = strings.ReplaceAll(p, ".", `\.`)
	}
	// The match is a FUZZY one — no anchors — matching the upstream product,
	// whose create dialog states "the rule is matched loosely". A rule therefore
	// hits anywhere in the uploaded file name, which catches the classic
	// double-extension smuggle (`shell.php.jpg`) for free but also refuses a name
	// that merely contains the text (`sh` refuses `shanghai.jpg`). That is a real
	// trade and the panel says so where the operator writes the rule.
	//
	// The transformations matter as much as the pattern: `removeNulls` defeats
	// `shell.php%00.jpg`, and `urlDecodeUni` defeats a percent-encoded dot. They
	// are applied in order after `t:none` clears any inherited default.
	return fmt.Sprintf(
		"SecRule FILES \"@rx (%s)\" \"id:%d,phase:2,deny,status:403,log,t:none,t:urlDecodeUni,t:removeNulls,t:lowercase,msg:'1Panel-X: upload refused by an upload restriction rule'\"\n",
		strings.Join(parts, "|"), uploadRuleID)
}

// DefaultUploadRules is what the upstream product ships with. The control plane
// seeds these for a new site so its list looks the same, but leaves the whole
// restriction switched OFF: turning upload blocking on for a site that already
// accepts those uploads is an outage, and that has to be the operator's call.
var DefaultUploadRules = []string{"php", "jsp", "asp", "exe", "sh"}
