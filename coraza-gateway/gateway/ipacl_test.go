package gateway

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseIPEntryAcceptsAddressesAndCIDR(t *testing.T) {
	for _, ok := range []string{"1.2.3.4", "  10.0.0.0/8 ", "2001:db8::1", "2001:db8::/32"} {
		if _, err := parseIPEntry(ok); err != nil {
			t.Fatalf("expected %q to parse: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "not-an-ip", "1.2.3.4/33", "1.2.3.4/", "999.1.1.1", "1.2.3.0/8x"} {
		if _, err := parseIPEntry(bad); err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
}

func TestNewIPACLEmptyIsNil(t *testing.T) {
	acl, err := newIPACL(nil, nil)
	if err != nil || acl != nil {
		t.Fatalf("empty ACL must be nil: acl=%v err=%v", acl, err)
	}
	if !acl.empty() {
		t.Fatal("nil ACL must report empty")
	}
	if _, err := newIPACL([]string{"garbage"}, nil); err == nil {
		t.Fatal("invalid allow entry must fail closed")
	}
	if _, err := newIPACL(nil, []string{"1.2.3.4/40"}); err == nil {
		t.Fatal("invalid deny entry must fail closed")
	}
}

func TestIPACLDecisionPrecedence(t *testing.T) {
	acl, err := newIPACL([]string{"10.0.0.0/8"}, []string{"10.1.2.3", "192.168.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		ip   string
		want aclDecision
	}{
		{"10.1.2.3", aclDeny},     // deny wins even though 10/8 is allow-listed
		{"192.168.5.5", aclDeny},  // deny CIDR
		{"10.9.9.9", aclAllow},    // allow CIDR, not denied
		{"172.16.0.1", aclNormal}, // in neither list
	}
	for _, c := range cases {
		if got := acl.decide(net.ParseIP(c.ip)); got != c.want {
			t.Fatalf("decide(%s)=%d want %d", c.ip, got, c.want)
		}
	}
	if acl.decide(nil) != aclNormal {
		t.Fatal("nil IP must be aclNormal")
	}
}

func TestAllowOnlyDoesNotBlockOthers(t *testing.T) {
	// An allow list is a trusted-bypass, NOT a default-deny: an IP outside it is
	// still evaluated normally, never blocked.
	acl, err := newIPACL([]string{"10.0.0.0/8"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := acl.decide(net.ParseIP("203.0.113.7")); got != aclNormal {
		t.Fatalf("non-allow-listed IP must be aclNormal, got %d", got)
	}
}

func serveFrom(h http.Handler, remoteAddr, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = remoteAddr
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestDenyListBlocksBeforeEngineInBothModes(t *testing.T) {
	for _, mode := range []Mode{ModeBlock, ModeDetection} {
		reached := false
		acl, _ := newIPACL(nil, []string{"203.0.113.9"})
		h := buildGateway(t, mode, 1<<20, &reached).WithIPACL(acl)
		rr := serveFrom(h, "203.0.113.9:5555", "GET", "/products", "")
		if rr.Code != http.StatusForbidden || reached {
			t.Fatalf("[%s] denied IP must get 403 and never reach upstream: code=%d reached=%v", mode, rr.Code, reached)
		}
		if strings.Contains(rr.Body.String(), upstreamMarker) {
			t.Fatalf("[%s] deny page must not leak upstream body", mode)
		}
		// A different, clean IP still passes.
		reached = false
		rr = serveFrom(h, "198.51.100.4:5555", "GET", "/products", "")
		if rr.Code != http.StatusOK || !reached {
			t.Fatalf("[%s] non-denied IP must pass: code=%d reached=%v", mode, rr.Code, reached)
		}
	}
}

func TestAllowListBypassesEngineButStillProxies(t *testing.T) {
	reached := false
	acl, _ := newIPACL([]string{"198.51.100.0/24"}, nil)
	h := buildGateway(t, ModeBlock, 1<<20, &reached).WithIPACL(acl)

	// SQLi from a trusted IP is NOT inspected → proxied through to upstream.
	rr := serveFrom(h, "198.51.100.7:5555", "GET", sqliTarget, "")
	if rr.Code != http.StatusOK || !reached {
		t.Fatalf("trusted IP must bypass WAF and reach upstream: code=%d reached=%v", rr.Code, reached)
	}
	if !strings.Contains(rr.Body.String(), upstreamMarker) {
		t.Fatalf("trusted bypass must return upstream body, got %q", rr.Body.String())
	}

	// The SAME SQLi from an untrusted IP is still blocked (engine active).
	reached = false
	rr = serveFrom(h, "203.0.113.20:5555", "GET", sqliTarget, "")
	if rr.Code != http.StatusForbidden || reached {
		t.Fatalf("untrusted IP SQLi must still be blocked: code=%d reached=%v", rr.Code, reached)
	}
}

func TestAllowListStillEnforcesBodyLimit(t *testing.T) {
	// A trusted IP bypasses CRS inspection but the W3 body ceiling still applies.
	reached := false
	acl, _ := newIPACL([]string{"198.51.100.7"}, nil)
	h := buildGateway(t, ModeBlock, 512, &reached).WithIPACL(acl)
	rr := serveFrom(h, "198.51.100.7:5555", "POST", "/upload", strings.Repeat("a", 4096))
	if rr.Code != http.StatusRequestEntityTooLarge || reached {
		t.Fatalf("trusted oversize body must still be 413: code=%d reached=%v", rr.Code, reached)
	}
}

func TestParseConfigRejectsInvalidIPList(t *testing.T) {
	for _, bad := range []string{
		`{"sites":[{"host":"a","upstream":"http://x:1","denyIps":["not-an-ip"]}]}`,
		`{"sites":[{"host":"a","upstream":"http://x:1","allowIps":["10.0.0.0/40"]}]}`,
	} {
		if _, err := ParseConfig([]byte(bad)); err == nil {
			t.Errorf("config with invalid IP list must be rejected: %s", bad)
		}
	}
	good := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:8081","allowIps":["10.0.0.0/8"],"denyIps":["1.2.3.4","2001:db8::/32"]}]}`
	cfg, err := ParseConfig([]byte(good))
	if err != nil || len(cfg.Sites[0].DenyIPs) != 2 || len(cfg.Sites[0].AllowIPs) != 1 {
		t.Fatalf("valid IP lists should parse: err=%v site=%#v", err, cfg.Sites)
	}
}

func TestRouterEnforcesPerSiteACL(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ORIGIN")
	}))
	defer origin.Close()
	eng, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRouter(Config{Sites: []SiteConfig{
		{Host: "acl.example", Upstream: origin.URL, DenyIPs: []string{"203.0.113.0/24"}, AllowIPs: []string{"198.51.100.5"}},
	}}, eng, ModeBlock, "X-Real-IP")
	if err != nil {
		t.Fatal(err)
	}
	req := func(realIP, target string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://acl.example"+target, nil)
		r.Host = "acl.example"
		r.Header.Set("X-Real-IP", realIP)
		rr := httptest.NewRecorder()
		rt.ServeHTTP(rr, r)
		return rr
	}
	if rr := req("203.0.113.7", "/"); rr.Code != http.StatusForbidden || strings.Contains(rr.Body.String(), "ORIGIN") {
		t.Fatalf("denied real-IP must be 403: %d %q", rr.Code, rr.Body.String())
	}
	if rr := req("198.51.100.5", sqliTarget); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "ORIGIN") {
		t.Fatalf("trusted real-IP must bypass WAF to origin: %d %q", rr.Code, rr.Body.String())
	}
	if rr := req("192.0.2.50", sqliTarget); rr.Code != http.StatusForbidden {
		t.Fatalf("normal real-IP SQLi must still be blocked: %d", rr.Code)
	}
}

func TestNewRouterRejectsInvalidACLEntry(t *testing.T) {
	eng, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRouter(Config{Sites: []SiteConfig{
		{Host: "a.example", Upstream: "http://127.0.0.1:8081", DenyIPs: []string{"bogus"}},
	}}, eng, ModeBlock, ""); err == nil {
		t.Fatal("router must reject a site with an invalid ACL entry")
	}
}
