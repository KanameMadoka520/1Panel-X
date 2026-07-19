package wafconfig

import (
	"errors"
	"regexp"
	"strings"
)

var proxyPassPattern = regexp.MustCompile(`(?m)^(\s*proxy_pass\s+)([^;]+)(;\s*(?:#.*)?)$`)

// ReplaceProxyPass changes only the proxy_pass directive in an existing nginx
// location fragment. It preserves all headers, cache, SNI and WebSocket
// directives, which is safer than rebuilding the location from a template.
func ReplaceProxyPass(content, replacement string) (string, string, error) {
	replacement = strings.TrimSpace(replacement)
	if replacement == "" || strings.ContainsAny(replacement, ";\r\n") {
		return "", "", errors.New("invalid proxy_pass replacement")
	}
	matches := proxyPassPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) != 1 {
		return "", "", errors.New("expected exactly one proxy_pass directive")
	}
	m := matches[0]
	old := content[m[4]:m[5]]
	updated := content[:m[4]] + replacement + content[m[5]:]
	return updated, strings.TrimSpace(old), nil
}

// RestoreProxyPass restores a previously captured origin. It uses the same
// narrow replacement operation and therefore cannot erase unrelated directives.
func RestoreProxyPass(content, origin string) (string, error) {
	updated, _, err := ReplaceProxyPass(content, origin)
	return updated, err
}
