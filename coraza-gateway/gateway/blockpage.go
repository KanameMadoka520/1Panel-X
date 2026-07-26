package gateway

import "net/http"

// blockPageHTML is a static, generic block page. It deliberately contains no
// origin address, upstream error, rule id, or stack detail (W7).
const blockPageHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Request blocked</title></head>
<body style="font-family:system-ui,sans-serif;text-align:center;padding:3rem;color:#333">
<h1 style="font-size:2rem;margin:0 0 .5rem">403 — Request blocked</h1>
<p>Your request was blocked by the web application firewall.</p>
</body></html>
`

func writeBlockPage(w http.ResponseWriter) {
	_, _ = w.Write([]byte(blockPageHTML))
}

// writeForbidden answers a 403 with the generic block page. It is the single
// entry point for an explicit refusal (unknown Host W12, or an IP-denylist hit)
// so every operator-initiated block leaks the same zero origin detail (W7).
func writeForbidden(w http.ResponseWriter) {
	w.WriteHeader(http.StatusForbidden)
	writeBlockPage(w)
}

func writeRequestTooLarge(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_, _ = w.Write([]byte("request body too large\n"))
}

// blockWriter records the response status and whether a body was written, so the
// handler can attach a block page to a bodiless WAF interruption without
// clobbering a real upstream response.
type blockWriter struct {
	http.ResponseWriter
	status    int
	wroteBody bool
}

func (b *blockWriter) WriteHeader(code int) {
	b.status = code
	b.ResponseWriter.WriteHeader(code)
}

func (b *blockWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		b.wroteBody = true
	}
	return b.ResponseWriter.Write(p)
}
