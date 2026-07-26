package gateway

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewReverseProxy builds a hardened reverse proxy to origin: single-interpretation
// HTTP framing to the upstream (W2), no origin detail on error (W7), and stripped
// fingerprint response headers (W7).
func NewReverseProxy(origin *url.URL) *httputil.ReverseProxy {
	rp := httputil.NewSingleHostReverseProxy(origin)
	rp.Transport = &http.Transport{ForceAttemptHTTP2: false}
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusBadGateway)
	}
	rp.ModifyResponse = func(resp *http.Response) error {
		StripFingerprintHeaders(resp.Header)
		return nil
	}
	return rp
}

// Router fronts several protected sites on one gateway, dispatching each request
// to the right per-site WAF handler by Host. All sites share one engine/mode
// (per-site mode override is a future refinement). A request whose Host matches
// no configured site is DEFAULT-DENIED (W12) — never proxied — so a forged or
// unknown Host cannot bypass the WAF or reach an origin.
type Router struct {
	handlers map[string]http.Handler
	// engines is the number of distinct compiled policies this routing table
	// needed; kept so tests can assert that sites actually share instances.
	engines int
}

func NewRouter(cfg Config, engine *Engine, mode Mode, realIPHeader string) (*Router, error) {
	rt := &Router{handlers: make(map[string]http.Handler, len(cfg.Sites))}
	cache := newEngineCache(engine, enginePolicy{Mode: mode})
	for _, s := range cfg.Sites {
		host := normalizeHost(s.Host)
		if host == "" {
			return nil, fmt.Errorf("router: invalid host %q", s.Host)
		}
		if _, exists := rt.handlers[host]; exists {
			return nil, fmt.Errorf("router: duplicate normalized host %q", host)
		}
		origin, err := url.Parse(s.Upstream)
		if err != nil || origin.Scheme == "" || origin.Host == "" ||
			(origin.Scheme != "http" && origin.Scheme != "https") {
			return nil, fmt.Errorf("router: site %q invalid HTTP(S) upstream %q", s.Host, s.Upstream)
		}
		modeForSite := mode
		if s.Mode != "" {
			modeForSite = s.Mode
		}
		policyEngine, err := cache.get(enginePolicy{Mode: modeForSite}, s.Host)
		if err != nil {
			return nil, fmt.Errorf("router: %w", err)
		}
		acl, err := newIPACL(s.AllowIPs, s.DenyIPs)
		if err != nil {
			return nil, fmt.Errorf("router: site %q %w", s.Host, err)
		}
		h := NewHandler(policyEngine, NewReverseProxy(origin), modeForSite).
			WithRealIPHeader(realIPHeader).
			WithIPACL(acl)
		rt.handlers[host] = h
	}
	rt.engines = cache.size()
	return rt, nil
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, ok := rt.handlers[normalizeHost(r.Host)]; ok {
		h.ServeHTTP(w, r)
		return
	}
	// W12: unknown/forged Host selects no site → deny, never bypass.
	writeForbidden(w)
}

// normalizeHost lower-cases a valid HTTP Host and removes an optional port.
// net.SplitHostPort handles bracketed IPv6 correctly; unbracketed strings with
// multiple colons are rejected because they are not valid HTTP Host values.
func normalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if h == "" {
		return ""
	}
	if ip := net.ParseIP(h); ip != nil {
		return h
	}
	if strings.HasPrefix(h, "[") {
		if host, port, err := net.SplitHostPort(h); err == nil {
			if !validPort(port) {
				return ""
			}
			return strings.TrimSpace(host)
		}
		if strings.HasSuffix(h, "]") {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(h, "["), "]"))
		}
		return ""
	}
	if strings.Count(h, ":") > 1 {
		return ""
	}
	if host, port, err := net.SplitHostPort(h); err == nil {
		if !validPort(port) {
			return ""
		}
		return strings.TrimSpace(host)
	}
	if i := strings.LastIndexByte(h, ':'); i >= 0 {
		port := h[i+1:]
		if port == "" {
			return ""
		}
		for _, c := range port {
			if c < '0' || c > '9' {
				return ""
			}
		}
		h = h[:i]
	}
	return strings.TrimSpace(h)
}

func validPort(port string) bool {
	if port == "" {
		return false
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
