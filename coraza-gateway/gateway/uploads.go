package gateway

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The upload-extension ban is implemented as OUR OWN rule, not as a knob on the
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
// anomaly score, so one banned extension is one refusal.
const (
	maxBannedExtensions = 64
	maxExtensionLength  = 15
)

// extensionPattern is a security control, not tidiness. The list is interpolated
// into a SecRule regular expression inside a quoted directive; a value carrying a
// quote, a newline or a regex metacharacter could terminate the directive and
// append another one, and `SecRuleEngine Off` would disable the WAF for every
// site sharing that compiled engine. Restricting the charset to alphanumerics
// means no escaping is needed and none can be forgotten.
var extensionPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,15}$`)

// normalizeExtensions validates, lower-cases, de-duplicates and sorts the banned
// upload extension list. A leading dot is accepted and stripped, because that is
// how operators habitually write extensions.
func normalizeExtensions(exts []string) ([]string, error) {
	if len(exts) == 0 {
		return nil, nil
	}
	if len(exts) > maxBannedExtensions {
		return nil, fmt.Errorf("too many banned upload extensions (%d), limit is %d", len(exts), maxBannedExtensions)
	}
	seen := make(map[string]struct{}, len(exts))
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.TrimSpace(e)
		e = strings.TrimPrefix(e, ".")
		if e == "" {
			continue
		}
		if !extensionPattern.MatchString(e) {
			return nil, fmt.Errorf("invalid upload extension %q (letters and digits only, at most %d characters)", e, maxExtensionLength)
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

// uploadRuleID is our own rule id. The rule set reserves 900000-999999 for
// itself, so we stay well clear of that whole space.
const uploadRuleID = 8000003

// uploadDenyDirective builds the SecRule for a canonical (already validated,
// space-separated) extension list. An empty list produces no rule at all, so a
// site that bans nothing pays nothing.
func uploadDenyDirective(canonical string) string {
	if canonical == "" {
		return ""
	}
	alternation := strings.ReplaceAll(canonical, " ", "|")
	// The match is deliberately `\.(ext)(\.|$)` rather than `\.(ext)$`: a file
	// named `shell.php.jpg` is the classic double-extension bypass, and a server
	// that hands such a name to a handler by any component is exactly what this
	// control exists to protect. It does mean `notes.sh.txt` is refused when `sh`
	// is banned; for a security control that trade is the right way round.
	//
	// The transformations matter as much as the pattern: `removeNulls` defeats
	// `shell.php%00.jpg`, and `urlDecodeUni` defeats a percent-encoded dot. They
	// are applied in order after `t:none` clears any inherited default.
	return fmt.Sprintf(
		"SecRule FILES \"@rx \\.(%s)(\\.|$)\" \"id:%d,phase:2,deny,status:403,log,t:none,t:urlDecodeUni,t:removeNulls,t:lowercase,msg:'1Panel-X: upload of a banned file extension'\"\n",
		alternation, uploadRuleID)
}
