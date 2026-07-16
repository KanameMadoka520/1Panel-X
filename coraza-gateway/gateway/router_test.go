package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	good := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:8081"},{"host":"b.example","upstream":"http://127.0.0.1:8082"}]}`
	cfg, err := ParseConfig([]byte(good))
	if err != nil || len(cfg.Sites) != 2 {
		t.Fatalf("good config should parse 2 sites: err=%v sites=%d", err, len(cfg.Sites))
	}

	for _, bad := range []string{
		`{`, // invalid json
		`{"sites":[{"host":"","upstream":"http://x:1"}]}`, // empty host
		`{"sites":[{"host":"a","upstream":"not a url"}]}`, // upstream not absolute
		`{"sites":[{"host":"a","upstream":""}]}`,          // empty upstream
	} {
		if _, err := ParseConfig([]byte(bad)); err == nil {
			t.Errorf("should reject config: %s", bad)
		}
	}
}

func TestRouterDispatchesByHostAndDeniesUnknown(t *testing.T) {
	upA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "UPSTREAM-A")
	}))
	defer upA.Close()
	upB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "UPSTREAM-B")
	}))
	defer upB.Close()

	cfg := Config{Sites: []SiteConfig{
		{Host: "a.example", Upstream: upA.URL},
		{Host: "b.example", Upstream: upB.URL},
	}}
	eng, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	rt, err := NewRouter(cfg, eng, ModeBlock, "")
	if err != nil {
		t.Fatalf("router: %v", err)
	}

	serveHost := func(host, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://"+host+target, nil)
		req.Host = host
		rr := httptest.NewRecorder()
		rt.ServeHTTP(rr, req)
		return rr
	}

	// Clean requests route to the right upstream by Host.
	if rr := serveHost("a.example", "/"); rr.Code != 200 || !strings.Contains(rr.Body.String(), "UPSTREAM-A") {
		t.Fatalf("a.example should reach upstream A: %d %q", rr.Code, rr.Body.String())
	}
	// A Host carrying a :port still normalizes to the site.
	if rr := serveHost("b.example:443", "/"); rr.Code != 200 || !strings.Contains(rr.Body.String(), "UPSTREAM-B") {
		t.Fatalf("b.example:443 should reach upstream B: %d %q", rr.Code, rr.Body.String())
	}
	// W12: an unknown/forged Host is denied, never proxied.
	if rr := serveHost("evil.example", "/"); rr.Code != 403 || strings.Contains(rr.Body.String(), "UPSTREAM") {
		t.Fatalf("unknown host must be denied: %d %q", rr.Code, rr.Body.String())
	}
	// The WAF still blocks attacks on a known host.
	if rr := serveHost("a.example", sqliTarget); rr.Code != 403 {
		t.Fatalf("attack on a known host should be WAF-blocked: %d", rr.Code)
	}
}
