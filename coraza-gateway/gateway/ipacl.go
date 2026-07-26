package gateway

import (
	"fmt"
	"net"
	"strings"
)

// aclDecision is the outcome of matching a client IP against a site's explicit
// operator ACL, evaluated BEFORE the CRS heuristic engine.
type aclDecision int

const (
	// aclNormal: no explicit rule matched — hand the request to the WAF engine.
	aclNormal aclDecision = iota
	// aclAllow: the client IP is trusted — skip CRS inspection but still proxy.
	aclAllow
	// aclDeny: the client IP is blocked — answer 403 and never proxy.
	aclDeny
)

// ipACL is a per-site explicit operator access-control list. It is intentionally
// separate from the CRS ruleset: deny/allow are hard operator decisions, not
// heuristics, so a denied IP is refused in BOTH detection and block mode. The
// detection/block posture only governs the CRS engine's fail-open learning mode.
//
// Precedence is deny-wins: an IP present in both lists is denied. An allow entry
// is a trusted BYPASS (skip inspection, still proxy), not a default-deny — an
// empty allow list never blocks anyone.
type ipACL struct {
	allow []*net.IPNet
	deny  []*net.IPNet
}

func (a *ipACL) empty() bool {
	return a == nil || (len(a.allow) == 0 && len(a.deny) == 0)
}

func (a *ipACL) decide(ip net.IP) aclDecision {
	if a == nil || ip == nil {
		return aclNormal
	}
	if matchAnyNet(a.deny, ip) {
		return aclDeny
	}
	if len(a.allow) > 0 && matchAnyNet(a.allow, ip) {
		return aclAllow
	}
	return aclNormal
}

func matchAnyNet(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// newIPACL builds an ACL from allow/deny entries. Each entry is a single IPv4/
// IPv6 address or a CIDR block. An invalid entry is a hard error so a malformed
// config fails closed at load rather than silently dropping a deny rule.
func newIPACL(allow, deny []string) (*ipACL, error) {
	allowNets, err := parseIPNets(allow)
	if err != nil {
		return nil, fmt.Errorf("allow list: %w", err)
	}
	denyNets, err := parseIPNets(deny)
	if err != nil {
		return nil, fmt.Errorf("deny list: %w", err)
	}
	if len(allowNets) == 0 && len(denyNets) == 0 {
		return nil, nil
	}
	return &ipACL{allow: allowNets, deny: denyNets}, nil
}

func parseIPNets(entries []string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, len(entries))
	for _, raw := range entries {
		n, err := parseIPEntry(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// parseIPEntry accepts either a bare address ("1.2.3.4", "2001:db8::1") or a
// CIDR block ("10.0.0.0/8", "2001:db8::/32") and returns the covering network.
func parseIPEntry(raw string) (*net.IPNet, error) {
	entry := strings.TrimSpace(raw)
	if entry == "" {
		return nil, fmt.Errorf("empty entry")
	}
	if strings.Contains(entry, "/") {
		_, n, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", raw)
		}
		return n, nil
	}
	ip := net.ParseIP(entry)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP %q", raw)
	}
	bits := 32
	if ip.To4() == nil {
		bits = 128
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
}

// clientIP extracts the evaluated client IP from RemoteAddr, which applyRealIP
// has already rewritten from the trusted front-proxy header when configured.
func clientIP(remoteAddr string) net.IP {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(strings.TrimSpace(remoteAddr))
}
