package gateway

import (
	"net"
	"net/http"
	"strings"

	corazahttp "github.com/corazawaf/coraza/v3/http"
)

// Handler runs each request through the WAF, then (if allowed) the upstream
// reverse proxy. A blocked request never reaches the upstream and is answered
// with a generic block page that leaks no origin detail (W7).
type Handler struct {
	engine   *Engine
	upstream http.Handler
	mode     Mode
	// realIP recovers the true client address before evaluation, since the
	// gateway sits behind nginx on loopback and would otherwise see only the
	// proxy address. nil leaves the transport peer address in place.
	realIP *realIPResolver
	// acl is the per-site explicit operator IP allow/deny list, evaluated before
	// the CRS engine. nil means no ACL configured.
	acl *ipACL
	// lists is the panel-wide black/white list set (IP, IP group, URL,
	// User-Agent). It is shared by every site; the per-site acl above is the
	// narrower, additional control.
	lists *listMatcher
	// custom holds the operator-authored condition/action rules. They are
	// evaluated after the panel-wide lists and before the site's own IP list.
	custom *customMatcher
	// region is this site's geographic access control. nil means none configured;
	// it is never nil merely because the address database is missing — that case
	// fails the config instead.
	region *regionMatcher
	// site identifies which protected website this handler serves, so a decision
	// can be attributed to it in the enforcement journal.
	site siteRef
	// journal records non-CRS decisions. nil disables recording.
	journal *EventJournal
	// enforcer holds the process-wide ban table and rate-limit counters. It is
	// shared across config reloads so a policy save does not erase live state.
	enforcer *Enforcer
	// rateLimits are this site's configured frequency limits.
	rateLimits []RateLimitConfig
	// blockPage is the refusal response. nil falls back to the built-in page, so
	// a code path that forgets to attach one still refuses.
	blockPage *blockPage
	// challenger issues and verifies the interactive custom-rule actions. It is
	// process-wide so a config reload does not invalidate live clearances.
	challenger *challenger
}

func NewHandler(engine *Engine, upstream http.Handler, mode Mode) *Handler {
	return &Handler{engine: engine, upstream: upstream, mode: mode}
}

// WithLists attaches the panel-wide black/white lists. A nil or empty matcher
// is a no-op.
func (h *Handler) WithLists(m *listMatcher) *Handler {
	if !m.empty() {
		h.lists = m
	}
	return h
}

// WithCustomRules attaches the operator-authored rules. A nil or empty matcher
// is a no-op.
func (h *Handler) WithCustomRules(m *customMatcher) *Handler {
	if !m.empty() {
		h.custom = m
	}
	return h
}

// WithRegion attaches the site's geographic access control. A nil or empty
// matcher is a no-op.
func (h *Handler) WithRegion(m *regionMatcher) *Handler {
	if !m.empty() {
		h.region = m
	}
	return h
}

// WithBlockPage attaches the operator's refusal page.
func (h *Handler) WithBlockPage(p *blockPage) *Handler {
	h.blockPage = p
	return h
}

// WithChallenger attaches the interactive-challenge machinery.
func (h *Handler) WithChallenger(c *challenger) *Handler {
	h.challenger = c
	return h
}

// WithSite attaches the protected site's identity for event attribution.
func (h *Handler) WithSite(site siteRef) *Handler {
	h.site = site
	return h
}

// WithJournal attaches the enforcement-event journal and returns the handler for
// chaining. A nil journal is a no-op.
func (h *Handler) WithJournal(j *EventJournal) *Handler {
	h.journal = j
	return h
}

// WithEnforcer attaches the shared ban/rate-limit state and this site's limits.
func (h *Handler) WithEnforcer(e *Enforcer, limits []RateLimitConfig) *Handler {
	h.enforcer = e
	h.rateLimits = limits
	return h
}

// WithRealIPHeader configures the trusted real-client-IP header and returns the
// handler for chaining.
func (h *Handler) WithRealIPHeader(header string) *Handler {
	h.realIP = newRealIPResolver(nil, header)
	return h
}

// WithRealIP attaches an explicit client-address recovery policy.
func (h *Handler) WithRealIP(r *realIPResolver) *Handler {
	h.realIP = r
	return h
}

// WithIPACL attaches an explicit operator IP allow/deny list and returns the
// handler for chaining. A nil or empty ACL is a no-op.
func (h *Handler) WithIPACL(acl *ipACL) *Handler {
	if !acl.empty() {
		h.acl = acl
	}
	return h
}

// applyRealIP rewrites RemoteAddr from the configured source so the engine
// evaluates and logs the true client address, not the nginx loopback address.
//
// A source that yields nothing leaves RemoteAddr alone. That is deliberate: the
// transport peer address is the one value no client can choose, so it is the
// only safe thing to fall back to.
func (h *Handler) applyRealIP(r *http.Request) {
	ip := h.realIP.resolve(r)
	if ip == "" {
		return
	}
	r.RemoteAddr = net.JoinHostPort(ip, "0")
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.applyRealIP(r)
	// The challenge endpoint is answered before every other check. It has to be:
	// the answer is a POST to the very site whose rule demanded the challenge,
	// and running it through that rule again would refuse the answer forever.
	if h.challenger != nil && strings.HasPrefix(r.URL.Path, challengePathPrefix) {
		h.challenger.handleVerify(w, r)
		return
	}
	// The explicit operator ACL is evaluated before anything else: a denied IP is
	// refused with the same generic 403 as an unknown Host, regardless of mode,
	// and never reaches the body reader or the CRS engine.
	// Explicit operator lists are evaluated before anything else, and an explicit
	// DENY outranks an explicit ALLOW. Both outrank the automatic mechanisms
	// (bans, rate limits, CRS): an allow entry is an exemption from automatic
	// machinery, never a licence to override another explicit refusal.
	//
	// The panel-wide lists are checked first, then the operator's custom rules,
	// then the site's own IP list, so a site-level allow cannot re-admit a client
	// the panel refused globally.
	clientAddr := clientIP(r.RemoteAddr)
	decision := aclNormal
	rule := ""
	if h.lists != nil {
		decision, rule = h.lists.decide(r, clientAddr)
	}
	if decision == aclNormal && h.custom != nil {
		out := h.custom.decide(r, clientAddr)
		// A `log` rule records regardless of what the request goes on to do; that
		// is the whole point of being able to watch a rule before arming it.
		if out.Observed != "" {
			h.recordEvent(r, EventCustomRule, out.Observed, "detected")
		}
		if out.Decision != aclNormal {
			decision, rule = out.Decision, out.Rule
		}
		// A challenge holds the request rather than refusing or exempting it. A
		// visitor who already carries a clearance falls straight through to the
		// rest of the pipeline — the challenge admits them, it does not exempt
		// them from the rule set.
		if out.Challenge != "" && !h.challenger.cleared(r, out.Challenge) {
			h.recordEvent(r, EventChallenge, out.Rule, "challenged")
			if h.challenger == nil {
				// Nothing can issue the challenge, so the only honest answer is a
				// refusal. Admitting the request would enforce nothing at all.
				h.blockPage.orDefault().write(w, r)
				return
			}
			if out.Challenge == challengeCaptcha {
				h.challenger.serveCaptcha(w, r, "")
			} else {
				h.challenger.serveJSChallenge(w, r)
			}
			return
		}
	}
	if decision == aclNormal && h.acl != nil {
		decision = h.acl.decide(clientAddr)
		switch decision {
		case aclDeny:
			rule = "site:ip-deny"
		case aclAllow:
			rule = "site:ip-allow"
		}
	}
	if decision == aclDeny {
		if rule == "" {
			rule = "ip-deny-list"
		}
		kind := EventACLDeny
		if strings.HasPrefix(rule, "custom:") {
			kind = EventCustomRule
		}
		h.recordEvent(r, kind, rule, "blocked")
		h.blockPage.orDefault().write(w, r)
		return
	}
	// Content-Length is rejected before Coraza reads the stream. Production nginx
	// also enforces the same 13 MiB ceiling and returns its stable public 413.
	if h.rejectOversizeBody(w, r) {
		h.recordEvent(r, EventOversize, "body-limit", "blocked")
		return
	}
	// A trusted (allow-listed) client bypasses CRS inspection but is still proxied
	// through the same hardened reverse proxy. The operator's explicit exemption
	// also outranks bans and frequency limits — those are automatic heuristics,
	// this is a deliberate decision — so it is evaluated before both.
	if decision == aclAllow {
		h.serveTrusted(w, r)
		return
	}
	// Region control sits AFTER the explicit allow shortcut above — an operator
	// exemption outranks a geographic policy, the same way it outranks bans and
	// frequency limits — and before the automatic mechanisms, because it is
	// itself a deliberate decision rather than a heuristic.
	if h.region != nil {
		if refused, country := h.region.refuses(clientAddr); refused {
			label := "region"
			if country != "" {
				label += ":" + country
			}
			h.recordEvent(r, EventRegion, label, "blocked")
			h.blockPage.orDefault().write(w, r)
			return
		}
	}
	// A client already banned is refused before any counting: letting a banned
	// client keep feeding the counters that banned it would extend its own ban
	// for as long as it keeps knocking.
	if entry, ok := h.enforcer.Banned(clientIPString(r.RemoteAddr)); ok {
		h.recordEvent(r, EventBanned, "ratelimit:"+string(entry.Kind), "blocked")
		h.blockPage.orDefault().write(w, r)
		return
	}
	if out := h.enforcer.CountRequest(h.site, h.rateLimits, r); out.Triggered {
		h.recordEvent(r, EventRateLimit, out.Rule, outcomeAction(out))
		if out.Banned {
			h.blockPage.orDefault().write(w, r)
			return
		}
	}
	// ran records whether the upstream was actually invoked, so we can tell a
	// WAF interruption (upstream never ran) from a real upstream 4xx/5xx, and so
	// the panic path (W1) knows whether it may fail open.
	ran := false
	defer func() {
		if rec := recover(); rec != nil {
			h.onPanic(w, r, ran)
		}
	}()

	marker := http.HandlerFunc(func(uw http.ResponseWriter, ur *http.Request) {
		ran = true
		h.upstream.ServeHTTP(uw, ur)
	})

	bw := &blockWriter{ResponseWriter: w}
	// WrapHandler is Coraza's vetted phase driver: it evaluates the request and,
	// on a disruptive match in block mode, writes the interruption status and
	// does NOT call marker. We reuse it rather than hand-driving phases so there
	// is no bespoke bypass surface.
	corazahttp.WrapHandler(h.engine.WAF(), marker).ServeHTTP(bw, r)

	// A rule-set block leaves an empty-bodied 4xx/5xx with the upstream untouched
	// — give it the block page. The status the engine chose is kept: it is the
	// interruption's own decision, and overriding it here would misreport what
	// actually happened.
	if !ran && bw.status >= 400 && !bw.wroteBody {
		h.blockPage.orDefault().writeBody(bw, r)
	}

	// Response-status limits are counted after the fact. A ban installed here
	// cannot retroactively refuse THIS request — the response is already on the
	// wire — it takes effect from the next one, which is exactly how a scanner
	// tripping a 404 threshold is supposed to behave.
	if out := h.enforcer.CountStatus(h.site, h.rateLimits, r, bw.status); out.Triggered {
		h.recordEvent(r, EventRateLimit, out.Rule, outcomeAction(out))
	}
}

// outcomeAction reports what actually happened, not what was configured: a limit
// with no ban duration is recorded as detected because the request went through.
func outcomeAction(out rateLimitOutcome) string {
	if out.Banned {
		return "blocked"
	}
	return "detected"
}

// serveTrusted forwards an allow-listed request straight to the origin without
// CRS inspection. It keeps a recover guard so an upstream panic still fails
// closed (generic 403) rather than crashing the connection.
func (h *Handler) serveTrusted(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			h.blockPage.orDefault().write(w, r)
		}
	}()
	h.upstream.ServeHTTP(w, r)
}

func (h *Handler) rejectOversizeBody(w http.ResponseWriter, r *http.Request) bool {
	if h.engine.bodyLimit <= 0 || r.Body == nil {
		return false
	}
	if r.ContentLength > int64(h.engine.bodyLimit) {
		writeRequestTooLarge(w)
		return true
	}
	return false
}

// onPanic implements the W1 recover policy. A per-request evaluation panic must
// never silently pass an uninspected request in protect mode:
//   - block mode          → fail CLOSED (generic 403).
//   - detection mode, and the panic happened before the upstream ran → fail OPEN
//     (proxy through) — this is the explicitly-labelled learning posture only.
//   - detection mode but the upstream had already started → cannot safely resume,
//     so fail closed.
func (h *Handler) onPanic(w http.ResponseWriter, r *http.Request, ran bool) {
	if h.mode == ModeDetection && !ran {
		defer func() { _ = recover() }() // never double-panic out of recovery
		h.upstream.ServeHTTP(w, r)
		return
	}
	w.WriteHeader(http.StatusForbidden)
	h.blockPage.orDefault().writeBody(w, r)
}
