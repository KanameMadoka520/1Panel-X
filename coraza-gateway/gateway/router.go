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
	// journal records decisions the router itself makes, i.e. unknown Host.
	journal *EventJournal
}

func NewRouter(cfg Config, engine *Engine, mode Mode, realIPHeader string) (*Router, error) {
	return NewRouterWithJournal(cfg, engine, mode, realIPHeader, nil)
}

func NewRouterWithJournal(cfg Config, engine *Engine, mode Mode, realIPHeader string, journal *EventJournal) (*Router, error) {
	return NewRouterWithEnforcer(cfg, engine, mode, realIPHeader, journal, nil)
}

// NewRouterWithEnforcer builds the routing table over shared enforcement state.
// The enforcer is owned by the process, not by the routing table, so rebuilding
// the table on a config reload does not reset live bans or counters.
func NewRouterWithEnforcer(cfg Config, engine *Engine, mode Mode, realIPHeader string, journal *EventJournal, enforcer *Enforcer) (*Router, error) {
	return NewRouterWithGeo(cfg, engine, mode, realIPHeader, journal, enforcer, nil)
}

// NewRouterWithGeo builds the routing table with an address database available
// for region access control. A site that configures a region policy without a
// database is a hard error, so the gateway can never report itself ready while
// silently enforcing none of it.
func NewRouterWithGeo(cfg Config, engine *Engine, mode Mode, realIPHeader string, journal *EventJournal, enforcer *Enforcer, geo *GeoDB) (*Router, error) {
	rt := &Router{handlers: make(map[string]http.Handler, len(cfg.Sites)), journal: journal}
	// The attack limit lives on the process-wide enforcer, so a reload updates the
	// threshold without discarding the counts accumulated so far.
	enforcer.SetAttackLimit(cfg.AttackRateLimit)
	// One compiled list set is shared by every site: the lists are panel-wide, so
	// compiling them per site would multiply identical regexes by the site count.
	lists, err := newListMatcher(cfg.Lists, cfg.IPGroups)
	if err != nil {
		return nil, fmt.Errorf("router: %w", err)
	}
	custom, err := newCustomMatcher(cfg.CustomRules)
	if err != nil {
		return nil, fmt.Errorf("router: %w", err)
	}
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
		sitePolicy, err := s.enginePolicy(mode)
		if err != nil {
			return nil, fmt.Errorf("router: site %q %w", s.Host, err)
		}
		policyEngine, err := cache.get(sitePolicy, s.Host)
		if err != nil {
			return nil, fmt.Errorf("router: %w", err)
		}
		acl, err := newIPACL(s.AllowIPs, s.DenyIPs)
		if err != nil {
			return nil, fmt.Errorf("router: site %q %w", s.Host, err)
		}
		region, err := newRegionMatcher(s.Region, geo, host)
		if err != nil {
			return nil, fmt.Errorf("router: %w", err)
		}
		h := NewHandler(policyEngine, NewReverseProxy(origin), modeForSite).
			WithRealIPHeader(realIPHeader).
			WithIPACL(acl).
			WithLists(lists).
			WithCustomRules(custom).
			WithRegion(region).
			WithSite(siteRef{WebsiteID: s.WebsiteID, Host: host}).
			WithJournal(journal).
			WithEnforcer(enforcer, s.RateLimits)
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
	// W12: unknown/forged Host selects no site → deny, never bypass. The request
	// belongs to no website, so the record carries no website id — the control
	// plane keeps these in an explicit "unattributed" bucket rather than folding
	// them into an arbitrary site.
	rt.journal.Record(EnforcementEvent{
		Kind:     EventUnknownHost,
		Host:     truncateField(r.Host),
		ClientIP: clientIPString(r.RemoteAddr),
		Method:   r.Method,
		URI:      truncateField(r.URL.RequestURI()),
		Rule:     "unknown-host",
		Action:   "blocked",
	})
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
