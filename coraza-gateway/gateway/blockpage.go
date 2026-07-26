package gateway

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

// defaultBlockPageHTML is the built-in refusal page. It deliberately contains no
// origin address, upstream error, rule id, or stack detail (W7).
const defaultBlockPageHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Request blocked</title></head>
<body style="font-family:system-ui,sans-serif;text-align:center;padding:3rem;color:#333">
<h1 style="font-size:2rem;margin:0 0 .5rem">403 — Request blocked</h1>
<p>Your request was blocked by the web application firewall.</p>
</body></html>
`

const (
	// maxBlockPageBytes bounds the operator's page. It is served on every refusal,
	// so an unbounded one would turn a flood of blocks into a bandwidth problem of
	// its own.
	maxBlockPageBytes = 64 << 10
	// blockPagePlaceholderIP and ...Time are the only substitutions offered.
	// Everything else stays literal: the page is operator-authored HTML, and the
	// fewer things this code splices into it, the fewer ways it can go wrong.
	blockPagePlaceholderIP   = "{{ip}}"
	blockPagePlaceholderTime = "{{time}}"
)

// BlockPage is the operator's custom refusal page.
type BlockPage struct {
	// Status is the HTTP status served on a refusal. Zero means 403.
	Status int `json:"status,omitempty"`
	// HTML is the page body. Empty keeps the built-in page.
	HTML string `json:"html,omitempty"`
}

// allowedBlockStatuses are the statuses a refusal may carry.
//
// The set is closed on purpose. A 5xx would blame the origin for a decision the
// WAF made, a 3xx would turn a refusal into a redirect the operator would then
// have to secure, and an arbitrary number would let a typo produce a response no
// client knows how to read.
var allowedBlockStatuses = map[int]struct{}{
	http.StatusOK:        {},
	http.StatusForbidden: {},
	http.StatusNotFound:  {},
}

func (p *BlockPage) validate() error {
	if p == nil {
		return nil
	}
	if len(p.HTML) > maxBlockPageBytes {
		return fmt.Errorf("block page is %d bytes, limit is %d", len(p.HTML), maxBlockPageBytes)
	}
	if p.Status == 0 {
		return nil
	}
	if _, ok := allowedBlockStatuses[p.Status]; !ok {
		return fmt.Errorf("block page status %d is not one of 200, 403 or 404", p.Status)
	}
	return nil
}

// blockPage is the compiled refusal response.
type blockPage struct {
	status   int
	template string
}

func newBlockPage(p *BlockPage) *blockPage {
	out := &blockPage{status: http.StatusForbidden, template: defaultBlockPageHTML}
	if p == nil {
		return out
	}
	if p.Status != 0 {
		out.status = p.Status
	}
	if strings.TrimSpace(p.HTML) != "" {
		out.template = p.HTML
	}
	return out
}

// render substitutes the two supported placeholders.
//
// Both substituted values are HTML-ESCAPED. The client address in particular
// reaches us through the trusted real-IP header, and an unescaped value there
// would let whoever can set that header inject markup or script into a page the
// operator's own visitors are shown.
func (b *blockPage) render(clientIP string, now time.Time) string {
	page := b.template
	if strings.Contains(page, blockPagePlaceholderIP) {
		page = strings.ReplaceAll(page, blockPagePlaceholderIP, html.EscapeString(clientIP))
	}
	if strings.Contains(page, blockPagePlaceholderTime) {
		page = strings.ReplaceAll(page, blockPagePlaceholderTime, html.EscapeString(now.UTC().Format(time.RFC3339)))
	}
	return page
}

// write emits the refusal with the configured status.
func (b *blockPage) write(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(b.statusOr(http.StatusForbidden))
	b.writeBody(w, r)
}

// writeBody emits only the body, for the case where a status has already been
// written (a rule-set interruption sets its own).
func (b *blockPage) writeBody(w http.ResponseWriter, r *http.Request) {
	ip := ""
	if r != nil {
		ip = clientIPString(r.RemoteAddr)
	}
	_, _ = w.Write([]byte(b.render(ip, time.Now())))
}

func (b *blockPage) statusOr(fallback int) int {
	if b == nil || b.status == 0 {
		return fallback
	}
	return b.status
}

// defaultBlockPage is used wherever no configuration has been attached, so a
// code path that forgets to thread one through still refuses rather than panics.
var defaultBlockPage = &blockPage{status: http.StatusForbidden, template: defaultBlockPageHTML}

func (b *blockPage) orDefault() *blockPage {
	if b == nil {
		return defaultBlockPage
	}
	return b
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
