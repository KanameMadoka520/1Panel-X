package wafconfig

import (
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
)

var proxyPassPattern = regexp.MustCompile(`(?m)^(\s*proxy_pass\s+)([^;]+)(;\s*(?:#.*)?)$`)

const managedDirectiveMarker = "# 1panel-x-waf-previous="

func EnsureManagedDirective(content, name, value string) (string, error) {
	pattern, err := directivePattern(name)
	if err != nil {
		return "", err
	}
	matches := pattern.FindAllStringIndex(content, -1)
	if len(matches) > 1 {
		return "", errors.New("expected at most one " + name + " directive")
	}
	previous := ""
	if len(matches) == 1 {
		previous = content[matches[0][0]:matches[0][1]]
		content = content[:matches[0][0]] + content[matches[0][1]:]
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(previous))
	line := name + " " + value + "; " + managedDirectiveMarker + encoded
	proxyMatch := proxyPassPattern.FindStringSubmatchIndex(content)
	if proxyMatch == nil {
		return "", errors.New("proxy_pass directive not found")
	}
	proxyLine := content[proxyMatch[0]:proxyMatch[1]]
	indent := proxyLine[:len(proxyLine)-len(strings.TrimLeft(proxyLine, " \t"))]
	return content[:proxyMatch[0]] + indent + line + "\n" + content[proxyMatch[0]:], nil
}

func RestoreManagedDirective(content, name string) (string, error) {
	pattern, err := directivePattern(name)
	if err != nil {
		return "", err
	}
	matches := pattern.FindAllStringIndex(content, -1)
	if len(matches) != 1 {
		return "", errors.New("expected exactly one managed " + name + " directive")
	}
	line := content[matches[0][0]:matches[0][1]]
	markerAt := strings.Index(line, managedDirectiveMarker)
	if markerAt < 0 {
		return "", errors.New("managed " + name + " marker not found")
	}
	encoded := strings.TrimSpace(line[markerAt+len(managedDirectiveMarker):])
	previous, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("invalid managed " + name + " marker")
	}
	return content[:matches[0][0]] + string(previous) + content[matches[0][1]:], nil
}

func directivePattern(name string) (*regexp.Regexp, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, ";\r\n{}") {
		return nil, errors.New("invalid nginx directive")
	}
	return regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(name) + `[ \t]+[^;]+;[^\r\n]*(?:\r?\n)?`), nil
}

func EnsureDirective(content, name, value string) (string, error) {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if name == "" || value == "" || strings.ContainsAny(name+value, ";\r\n{}") {
		return "", errors.New("invalid nginx directive")
	}
	pattern := regexp.MustCompile(`(?m)^(\s*)` + regexp.QuoteMeta(name) + `\s+[^;]+;\s*(?:#.*)?$`)
	matches := pattern.FindAllStringIndex(content, -1)
	if len(matches) > 1 {
		return "", errors.New("expected at most one " + name + " directive")
	}
	if len(matches) == 1 {
		line := pattern.FindStringSubmatch(content[matches[0][0]:matches[0][1]])
		replacement := line[1] + name + " " + value + ";"
		return content[:matches[0][0]] + replacement + content[matches[0][1]:], nil
	}
	proxyMatch := proxyPassPattern.FindStringSubmatchIndex(content)
	if proxyMatch == nil {
		return "", errors.New("proxy_pass directive not found")
	}
	proxyLine := content[proxyMatch[0]:proxyMatch[1]]
	indent := proxyLine[:len(proxyLine)-len(strings.TrimLeft(proxyLine, " \t"))]
	insertAt := proxyMatch[0]
	return content[:insertAt] + indent + name + " " + value + ";\n" + content[insertAt:], nil
}

func RemoveDirective(content, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, ";\r\n{}") {
		return "", errors.New("invalid nginx directive")
	}
	pattern := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(name) + `[ \t]+[^;]+;[ \t]*(?:#.*)?\r?\n?`)
	matches := pattern.FindAllStringIndex(content, -1)
	if len(matches) > 1 {
		return "", errors.New("expected at most one " + name + " directive")
	}
	if len(matches) == 0 {
		return content, nil
	}
	return content[:matches[0][0]] + content[matches[0][1]:], nil
}

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
