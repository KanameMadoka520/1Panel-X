package gateway

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// Recovering the true client address is the single most security-load-bearing
// piece of parsing in this gateway. Every explicit control keys off it — IP
// allow/deny lists, bans, frequency counters, region policy — so an address a
// client can choose for itself defeats all of them at once.
//
// The rule that shapes this file: NEVER take a value a client could have put
// there. X-Forwarded-For is appended to by each proxy, so its LEFTMOST entry is
// whatever the original caller wrote and is worthless; only entries counted from
// the RIGHT correspond to hops the infrastructure actually observed.

// RealIPMode names how the client address is recovered.
type RealIPMode string

const (
	// RealIPHeader reads one named header, set by the front proxy.
	RealIPHeader RealIPMode = "header"
	// RealIPHeaderList tries the well-known CDN headers in order and takes the
	// first that yields an address.
	RealIPHeaderList RealIPMode = "header-list"
	// RealIPXFF1/2/3 take the 1st, 2nd or 3rd entry counted from the RIGHT of
	// X-Forwarded-For, i.e. one, two or three proxy hops upstream of us.
	RealIPXFF1 RealIPMode = "xff-1"
	RealIPXFF2 RealIPMode = "xff-2"
	RealIPXFF3 RealIPMode = "xff-3"
)

// cdnRealIPHeaders are the headers commonly used by CDNs to carry the original
// client address, tried in this order.
//
// The order matters and is not alphabetical: the widely-standardised ones come
// first, then vendor-specific ones. A CDN that sets several must resolve to the
// same client, so the first hit is as good as any.
var cdnRealIPHeaders = []string{
	"x-forwarded-for",
	"x-real-ip",
	"x-forwarded",
	"forwarded-for",
	"forwarded",
	"true-client-ip",
	"client-ip",
	"ali-cdn-real-ip",
	"cdn-src-ip",
	"cdn-real-ip",
	"cf-connecting-ip",
	"x-cluster-client-ip",
	"wl-proxy-client-ip",
	"proxy-client-ip",
}

// CDNRealIPHeaders exposes the list so the control plane can show operators
// exactly what will be read, rather than describing it in prose that could drift.
func CDNRealIPHeaders() []string {
	out := make([]string, len(cdnRealIPHeaders))
	copy(out, cdnRealIPHeaders)
	return out
}

// headerNamePattern bounds a configured header name. Header names are tokens by
// definition; anything else is a typo or an attempt at something else.
var headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// RealIPConfig is one site's client-address recovery policy.
type RealIPConfig struct {
	Mode RealIPMode `json:"mode,omitempty"`
	// Header names the single header read in RealIPHeader mode.
	Header string `json:"header,omitempty"`
}

func (c *RealIPConfig) validate() error {
	if c == nil {
		return nil
	}
	switch c.Mode {
	case RealIPHeaderList, RealIPXFF1, RealIPXFF2, RealIPXFF3:
		if strings.TrimSpace(c.Header) != "" {
			return fmt.Errorf("mode %q does not take a header name", c.Mode)
		}
		return nil
	case RealIPHeader:
		if !headerNamePattern.MatchString(strings.TrimSpace(c.Header)) {
			return fmt.Errorf("mode %q needs a valid header name, got %q", c.Mode, c.Header)
		}
		c.Header = strings.TrimSpace(c.Header)
		return nil
	case "":
		return nil
	default:
		return fmt.Errorf("unknown real-ip mode %q", c.Mode)
	}
}

// realIPResolver recovers the client address for one site.
type realIPResolver struct {
	mode RealIPMode
	// header is the single header for RealIPHeader mode.
	header string
	// depth is how many entries from the RIGHT of X-Forwarded-For to skip past,
	// 1 meaning the rightmost entry.
	depth int
}

// newRealIPResolver builds the resolver. A nil config falls back to the
// process-wide default header, which is what the front proxy sets.
func newRealIPResolver(c *RealIPConfig, defaultHeader string) *realIPResolver {
	if c == nil || c.Mode == "" {
		if strings.TrimSpace(defaultHeader) == "" {
			return nil
		}
		return &realIPResolver{mode: RealIPHeader, header: defaultHeader}
	}
	switch c.Mode {
	case RealIPXFF1:
		return &realIPResolver{mode: RealIPXFF1, depth: 1}
	case RealIPXFF2:
		return &realIPResolver{mode: RealIPXFF2, depth: 2}
	case RealIPXFF3:
		return &realIPResolver{mode: RealIPXFF3, depth: 3}
	case RealIPHeaderList:
		return &realIPResolver{mode: RealIPHeaderList}
	default:
		return &realIPResolver{mode: RealIPHeader, header: c.Header}
	}
}

// resolve returns the client address, or "" to keep the transport peer address.
//
// Returning "" is the safe outcome everywhere: it leaves RemoteAddr as the
// address the kernel actually saw, which is never forgeable.
func (r *realIPResolver) resolve(req *http.Request) string {
	if r == nil {
		return ""
	}
	switch r.mode {
	case RealIPXFF1, RealIPXFF2, RealIPXFF3:
		return nthFromRight(req.Header.Get("X-Forwarded-For"), r.depth)
	case RealIPHeaderList:
		for _, name := range cdnRealIPHeaders {
			v := strings.TrimSpace(req.Header.Get(name))
			if v == "" {
				continue
			}
			// A list-valued header (X-Forwarded-For, Forwarded) carries the CDN's
			// view of the original caller in its FIRST entry. This mode exists for
			// the topology where a CDN is the only ingress and is trusted to have
			// overwritten it; that trust is the operator's to grant, and the UI
			// says so.
			if ip := firstAddress(v); ip != "" {
				return ip
			}
		}
		return ""
	default:
		v := strings.TrimSpace(req.Header.Get(r.header))
		if v == "" {
			return ""
		}
		return firstAddress(v)
	}
}

// nthFromRight returns the nth entry counted from the right of a comma list.
//
// If the list is shorter than n, the answer is "" and the caller keeps the
// transport peer address. Falling back to some other entry would hand the
// decision to whoever sent the shortest list — i.e. to the attacker.
func nthFromRight(value string, n int) string {
	if n <= 0 {
		return ""
	}
	parts := strings.Split(value, ",")
	if len(parts) < n {
		return ""
	}
	candidate := strings.TrimSpace(parts[len(parts)-n])
	return parseClientAddress(candidate)
}

func firstAddress(value string) string {
	first := strings.TrimSpace(strings.Split(value, ",")[0])
	return parseClientAddress(first)
}

// parseClientAddress accepts a bare address or an address:port, and rejects
// anything else. A value that is not an address must never become one.
func parseClientAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// `Forwarded: for=1.2.3.4` and quoted forms appear in the wild; accept the
	// bare address form only, and let anything else fall through to the peer.
	value = strings.TrimPrefix(strings.ToLower(value), "for=")
	value = strings.Trim(value, `"`)
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
			return ip.String()
		}
	}
	return ""
}
