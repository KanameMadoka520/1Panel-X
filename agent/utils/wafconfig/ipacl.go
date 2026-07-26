package wafconfig

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// MaxIPListEntries caps how many entries an allow or deny list may hold, so a
// single site's policy cannot bloat the gateway config or the control-plane row.
const MaxIPListEntries = 512

// NormalizeIPList validates, canonicalizes, de-duplicates, and sorts an IP
// allow/deny list. Each entry is a single IPv4/IPv6 address or a CIDR block.
// Canonicalization (lowercased IPv6, masked CIDR network) plus a stable sort
// makes the generated gateway config deterministic, so the config generation
// hash only changes when the effective rule set changes. An invalid entry is a
// hard error surfaced to the operator instead of silently dropped.
func NormalizeIPList(entries []string) ([]string, error) {
	seen := make(map[string]struct{}, len(entries))
	out := make([]string, 0, len(entries))
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		canonical, err := canonicalizeIPEntry(entry)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[canonical]; dup {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
	}
	if len(out) > MaxIPListEntries {
		return nil, fmt.Errorf("too many IP entries (%d), limit is %d", len(out), MaxIPListEntries)
	}
	sort.Strings(out)
	return out, nil
}

// MergeIPLists returns the canonical union of the panel-wide list and one
// site's list. Both sides were validated independently when saved, but the
// union is re-normalized (and re-capped) so the merged result honors the same
// MaxIPListEntries bound as a single list.
func MergeIPLists(global, site []string) ([]string, error) {
	merged := make([]string, 0, len(global)+len(site))
	merged = append(merged, global...)
	merged = append(merged, site...)
	return NormalizeIPList(merged)
}

func canonicalizeIPEntry(entry string) (string, error) {
	if strings.Contains(entry, "/") {
		_, n, err := net.ParseCIDR(entry)
		if err != nil {
			return "", fmt.Errorf("invalid CIDR %q", entry)
		}
		return n.String(), nil
	}
	ip := net.ParseIP(entry)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address %q", entry)
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), nil
	}
	return ip.String(), nil
}
