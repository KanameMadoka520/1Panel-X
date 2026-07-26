package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func resolveWith(t *testing.T, cfg *RealIPConfig, defaultHeader string, headers map[string]string, peer string) string {
	t.Helper()
	if err := cfg.validate(); err != nil {
		t.Fatalf("config rejected: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = peer
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h := NewHandler(nil, nil, ModeBlock).WithRealIP(newRealIPResolver(cfg, defaultHeader))
	h.applyRealIP(req)
	return clientIPString(req.RemoteAddr)
}

// THE load-bearing assertion of this file. X-Forwarded-For is appended to by
// each proxy, so its leftmost entry is whatever the original caller wrote.
// Taking it would let any client choose its own address and thereby defeat the
// IP allow/deny lists, bans, frequency counters and region policy at once.
func TestForwardedForIsCountedFromTheRight(t *testing.T) {
	// A client that forged two entries, then the real CDN edge appended by nginx.
	xff := "1.1.1.1, 2.2.2.2, 203.0.113.9"
	headers := map[string]string{"X-Forwarded-For": xff}

	got := resolveWith(t, &RealIPConfig{Mode: RealIPXFF1}, "", headers, "127.0.0.1:1234")
	if got != "203.0.113.9" {
		t.Fatalf("one hop up is the rightmost entry, got %q", got)
	}
	if got == "1.1.1.1" {
		t.Fatal("the leftmost entry is attacker-controlled and must never be taken")
	}
	if got := resolveWith(t, &RealIPConfig{Mode: RealIPXFF2}, "", headers, "127.0.0.1:1234"); got != "2.2.2.2" {
		t.Fatalf("two hops up is the second entry from the right, got %q", got)
	}
	if got := resolveWith(t, &RealIPConfig{Mode: RealIPXFF3}, "", headers, "127.0.0.1:1234"); got != "1.1.1.1" {
		t.Fatalf("three hops up is the third entry from the right, got %q", got)
	}
}

// A list shorter than the configured depth must keep the transport peer address.
// Falling back to some other entry would hand the choice to whoever sends the
// shortest list — that is, to the attacker.
func TestForwardedForShorterThanConfiguredDepthKeepsThePeer(t *testing.T) {
	headers := map[string]string{"X-Forwarded-For": "1.1.1.1"}
	got := resolveWith(t, &RealIPConfig{Mode: RealIPXFF3}, "", headers, "198.51.100.7:1234")
	if got != "198.51.100.7" {
		t.Fatalf("a too-short chain must keep the peer address, got %q", got)
	}
	// And an absent header likewise.
	if got := resolveWith(t, &RealIPConfig{Mode: RealIPXFF1}, "", nil, "198.51.100.7:1234"); got != "198.51.100.7" {
		t.Fatalf("no header must keep the peer address, got %q", got)
	}
}

func TestHeaderListTakesTheFirstHeaderThatYieldsAnAddress(t *testing.T) {
	// The first few are absent or unusable; cf-connecting-ip is the first hit.
	headers := map[string]string{
		"X-Forwarded":      "not-an-address",
		"CF-Connecting-IP": "203.0.113.9",
		"True-Client-IP":   "198.51.100.7",
	}
	got := resolveWith(t, &RealIPConfig{Mode: RealIPHeaderList}, "", headers, "127.0.0.1:1234")
	// true-client-ip comes before cf-connecting-ip in the list, so it wins.
	if got != "198.51.100.7" {
		t.Fatalf("the first header in list order that yields an address must win, got %q", got)
	}
	// Nothing usable anywhere keeps the peer.
	if got := resolveWith(t, &RealIPConfig{Mode: RealIPHeaderList}, "",
		map[string]string{"Client-IP": "nonsense"}, "192.0.2.9:1234"); got != "192.0.2.9" {
		t.Fatalf("no usable header must keep the peer address, got %q", got)
	}
}

func TestSingleHeaderModeAndDefault(t *testing.T) {
	got := resolveWith(t, &RealIPConfig{Mode: RealIPHeader, Header: "X-Real-IP"},
		"", map[string]string{"X-Real-IP": "203.0.113.9"}, "127.0.0.1:1234")
	if got != "203.0.113.9" {
		t.Fatalf("the named header must be read, got %q", got)
	}
	// No per-site config falls back to the process-wide header, which is what the
	// front proxy sets. This is the default deployment and must keep working.
	if got := resolveWith(t, nil, "X-Real-IP",
		map[string]string{"X-Real-IP": "203.0.113.9"}, "127.0.0.1:1234"); got != "203.0.113.9" {
		t.Fatalf("the process default header must still apply, got %q", got)
	}
	// A value that is not an address must never become one.
	if got := resolveWith(t, &RealIPConfig{Mode: RealIPHeader, Header: "X-Real-IP"},
		"", map[string]string{"X-Real-IP": "evil.example"}, "192.0.2.9:1234"); got != "192.0.2.9" {
		t.Fatalf("a non-address must keep the peer, got %q", got)
	}
}

func TestRealIPConfigValidation(t *testing.T) {
	bad := []*RealIPConfig{
		{Mode: "sideways"},
		{Mode: RealIPHeader},                              // needs a name
		{Mode: RealIPHeader, Header: "X-Real-IP: evil"},   // not a token
		{Mode: RealIPHeader, Header: "X\nReal"},           // newline
		{Mode: RealIPXFF1, Header: "X-Real-IP"},           // takes no name
		{Mode: RealIPHeaderList, Header: "cf-connecting"}, // takes no name
	}
	for _, c := range bad {
		if err := c.validate(); err == nil {
			t.Fatalf("config %+v must be rejected", c)
		}
	}
	body := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","realIp":{"mode":"sideways"}}]}`
	if _, err := ParseConfig([]byte(body)); err == nil {
		t.Fatal("an invalid real-ip policy must fail the whole config load")
	}
}

// The header list is shown to operators verbatim in the panel, so the two must
// come from the same place rather than from prose that could drift.
func TestCDNHeaderListIsExposedAndCopied(t *testing.T) {
	got := CDNRealIPHeaders()
	if len(got) != len(cdnRealIPHeaders) {
		t.Fatalf("the exposed list must match the one actually used: %d vs %d", len(got), len(cdnRealIPHeaders))
	}
	got[0] = "tampered"
	if cdnRealIPHeaders[0] == "tampered" {
		t.Fatal("the exposed list must be a copy, not the live one")
	}
}

// The whole point of recovering the address is that the explicit controls key
// off it. This asserts the wiring end to end: a deny list entry must match the
// address recovered from the configured source, not the loopback peer.
func TestRecoveredAddressDrivesTheDenyList(t *testing.T) {
	var reached bool
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sites: []SiteConfig{{
		Host:     "a.example",
		Upstream: "http://127.0.0.1:1",
		DenyIPs:  []string{"203.0.113.9"},
		RealIP:   &RealIPConfig{Mode: RealIPXFF1},
	}}}
	rt, err := NewRouter(cfg, engine, ModeBlock, "")
	if err != nil {
		t.Fatal(err)
	}
	rt.handlers["a.example"].(*Handler).upstream = recordingUpstream(&reached)

	reached = false
	rr := customRequest(rt, "GET", "/", map[string]string{"X-Forwarded-For": "1.1.1.1, 203.0.113.9"}, "127.0.0.1:1234")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("the denied address must be recovered from the chain, got %d", rr.Code)
	}
	// And a client forging the denied address on the LEFT must not thereby be
	// able to make someone else look denied, nor slip past by padding the chain.
	reached = false
	rr = customRequest(rt, "GET", "/", map[string]string{"X-Forwarded-For": "203.0.113.9, 198.51.100.7"}, "127.0.0.1:1234")
	if rr.Code != http.StatusOK || !reached {
		t.Fatalf("a forged leftmost entry must not decide the outcome, got %d reached=%v", rr.Code, reached)
	}
}
