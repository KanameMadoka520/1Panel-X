package wafconfig

import (
	"fmt"
	"regexp"
	"strings"
)

// Client-address recovery vocabulary, mirroring the data plane's contract.
//
// The security point behind the three X-Forwarded-For modes: that header is
// APPENDED to by each proxy, so its leftmost entry is whatever the original
// caller wrote. Only entries counted from the right correspond to hops the
// infrastructure actually observed, so the modes are expressed as "how many hops
// upstream", never as "the first value".
const (
	RealIPModeHeader     = "header"
	RealIPModeHeaderList = "header-list"
	RealIPModeXFF1       = "xff-1"
	RealIPModeXFF2       = "xff-2"
	RealIPModeXFF3       = "xff-3"
)

// CDNRealIPHeaders is the list tried, in order, by the header-list mode. It is
// exported so the panel shows operators exactly what will be read rather than
// describing it in prose that could drift from the code.
var CDNRealIPHeaders = []string{
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

var realIPHeaderPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// RealIPConfig is one site's client-address recovery policy.
type RealIPConfig struct {
	Mode string `json:"mode,omitempty"`
	// Header names the single header read in "header" mode.
	Header string `json:"header,omitempty"`
}

// IsZero reports "use the front proxy's header", which is emitted as absent.
func (c *RealIPConfig) IsZero() bool {
	return c == nil || strings.TrimSpace(c.Mode) == ""
}

// NormalizeRealIP validates and canonicalizes the policy.
func NormalizeRealIP(c RealIPConfig) (RealIPConfig, error) {
	c.Mode = strings.TrimSpace(c.Mode)
	c.Header = strings.TrimSpace(c.Header)
	switch c.Mode {
	case "":
		c.Header = ""
		return c, nil
	case RealIPModeHeaderList, RealIPModeXFF1, RealIPModeXFF2, RealIPModeXFF3:
		// These modes read a fixed source. Keeping a header name around would be a
		// value the operator can see but nothing reads.
		c.Header = ""
		return c, nil
	case RealIPModeHeader:
		if !realIPHeaderPattern.MatchString(c.Header) {
			return RealIPConfig{}, fmt.Errorf("this mode needs a valid header name, got %q", c.Header)
		}
		return c, nil
	default:
		return RealIPConfig{}, fmt.Errorf("unknown real IP mode %q", c.Mode)
	}
}
