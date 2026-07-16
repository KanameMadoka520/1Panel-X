package gateway

import (
	"fmt"
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
}

// NewRouter builds the per-site handler table from the routing config.
func NewRouter(cfg Config, engine *Engine, mode Mode, realIPHeader string) (*Router, error) {
	rt := &Router{handlers: make(map[string]http.Handler, len(cfg.Sites))}
	for _, s := range cfg.Sites {
		origin, err := url.Parse(s.Upstream)
		if err != nil || origin.Scheme == "" || origin.Host == "" {
			return nil, fmt.Errorf("router: site %q invalid upstream %q", s.Host, s.Upstream)
		}
		h := NewHandler(engine, NewReverseProxy(origin), mode).WithRealIPHeader(realIPHeader)
		rt.handlers[normalizeHost(s.Host)] = h
	}
	return rt, nil
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, ok := rt.handlers[normalizeHost(r.Host)]; ok {
		h.ServeHTTP(w, r)
		return
	}
	// W12: unknown/forged Host selects no site → deny, never bypass.
	w.WriteHeader(http.StatusForbidden)
	writeBlockPage(w)
}

// normalizeHost lower-cases and strips any :port so the routing key matches
// regardless of how the front proxy presents the Host header.
func normalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	return h
}
