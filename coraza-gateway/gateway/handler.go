package gateway

import (
	"net/http"

	corazahttp "github.com/corazawaf/coraza/v3/http"
)

// Handler runs each request through the WAF, then (if allowed) the upstream
// reverse proxy. A blocked request never reaches the upstream and is answered
// with a generic block page that leaks no origin detail (W7).
type Handler struct {
	engine   *Engine
	upstream http.Handler
	mode     Mode
}

func NewHandler(engine *Engine, upstream http.Handler, mode Mode) *Handler {
	return &Handler{engine: engine, upstream: upstream, mode: mode}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
