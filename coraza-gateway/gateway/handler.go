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
	// realIPHeader, when set, names a trusted header (e.g. "X-Real-IP") from
	// which to recover the true client IP before evaluation, since the gateway
	// sits behind nginx on loopback and would otherwise see only the proxy
	// address. Only safe because the sole supported topology is behind that
	// trusted proxy (W8/W12); empty disables it.
	realIPHeader string
	// acl is the per-site explicit operator IP allow/deny list, evaluated before
	// the CRS engine. nil means no ACL configured.
	acl *ipACL
}

func NewHandler(engine *Engine, upstream http.Handler, mode Mode) *Handler {
	return &Handler{engine: engine, upstream: upstream, mode: mode}
}

// WithRealIPHeader configures the trusted real-client-IP header and returns the
// handler for chaining.
func (h *Handler) WithRealIPHeader(header string) *Handler {
	h.realIPHeader = header
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

// applyRealIP rewrites RemoteAddr from the trusted header so Coraza evaluates and
// logs the true client IP, not the nginx loopback address. The leftmost value of
// a comma list is taken (the original client for X-Forwarded-For).
func (h *Handler) applyRealIP(r *http.Request) {
	if h.realIPHeader == "" {
		return
	}
	v := r.Header.Get(h.realIPHeader)
	if v == "" {
		return
	}
	ip := strings.TrimSpace(strings.Split(v, ",")[0])
	if net.ParseIP(ip) == nil {
		return
	}
	r.RemoteAddr = net.JoinHostPort(ip, "0")
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.applyRealIP(r)
	// The explicit operator ACL is evaluated before anything else: a denied IP is
	// refused with the same generic 403 as an unknown Host, regardless of mode,
	// and never reaches the body reader or the CRS engine.
	decision := aclNormal
	if h.acl != nil {
		decision = h.acl.decide(clientIP(r.RemoteAddr))
		if decision == aclDeny {
			writeForbidden(w)
			return
		}
	}
	// Content-Length is rejected before Coraza reads the stream. Production nginx
	// also enforces the same 13 MiB ceiling and returns its stable public 413.
	if h.rejectOversizeBody(w, r) {
		return
	}
	// A trusted (allow-listed) client bypasses CRS inspection but is still proxied
	// through the same hardened reverse proxy.
	if decision == aclAllow {
		h.serveTrusted(w, r)
		return
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

	// A WAF block leaves an empty-bodied 4xx/5xx with the upstream untouched —
	// give it a generic block page.
	if !ran && bw.status >= 400 && !bw.wroteBody {
		writeBlockPage(bw)
	}
}

// serveTrusted forwards an allow-listed request straight to the origin without
// CRS inspection. It keeps a recover guard so an upstream panic still fails
// closed (generic 403) rather than crashing the connection.
func (h *Handler) serveTrusted(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			writeForbidden(w)
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
	writeBlockPage(w)
}
