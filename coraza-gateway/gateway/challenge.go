package gateway

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Challenges are the two interactive custom-rule actions: a timed browser check
// and a CAPTCHA. Both end the same way — a signed clearance cookie — and both
// are stateless: the gateway keeps no per-visitor record, so a config reload or
// a restart cannot leave half a challenge dangling.
//
// What these are honestly worth:
//   - The timed check filters clients that do not run JavaScript and cannot wait.
//     The wait is enforced SERVER-side (a clearance presented too early is
//     refused), so scraping the token out of the page does not skip it. It is a
//     cost imposed on automation, not proof of a human.
//   - The CAPTCHA is self-contained and stops ordinary scripted traffic. It will
//     not stop a solving service, and the panel says so where it is switched on.
//
// Neither is a substitute for the rule set; they are what an operator reaches
// for when a path must stay reachable by people but not by tools.

const (
	// challengePathPrefix is reserved on every protected site. It is deliberately
	// unlikely to collide with a real application path.
	challengePathPrefix = "/__1panelx_waf/"
	challengeVerifyPath = challengePathPrefix + "verify"

	// clearanceCookie carries proof that a challenge was completed.
	clearanceCookie = "__1px_clearance"

	// challengeWait is how long the timed check makes a visitor wait. It is
	// enforced on this side, not by the page's own timer.
	challengeWait = 5 * time.Second
	// clearanceTTL is how long a completed challenge is honoured.
	clearanceTTL = 30 * time.Minute
	// challengeTokenTTL bounds how long an ISSUED challenge may be answered.
	challengeTokenTTL = 10 * time.Minute

	captchaLength = 4
)

// challengeKind names which challenge a rule demands. The kinds are kept apart
// in the signature so passing the cheap one never satisfies the expensive one.
type challengeKind string

const (
	challengeJS      challengeKind = "js"
	challengeCaptcha challengeKind = "captcha"
)

// challenger issues and verifies challenges.
type challenger struct {
	key []byte
	now func() time.Time
	// rand returns n random bytes; replaceable so a test can pin the captcha.
	rand func(n int) []byte
}

// processChallenger returns the one challenger this process uses.
//
// Its signing key must outlive the routing table: rebuilding it on every config
// reload would invalidate every clearance a visitor had already earned, so an
// unrelated policy save would send everyone back through the CAPTCHA.
var processChallenger = sync.OnceValue(newChallenger)

func newChallenger() *challenger {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		// A keyless challenger could not tell a forged clearance from a real one.
		// Returning nil makes every challenge rule fail the config instead.
		return nil
	}
	return &challenger{
		key: key,
		now: time.Now,
		rand: func(n int) []byte {
			buf := make([]byte, n)
			if _, err := rand.Read(buf); err != nil {
				return nil
			}
			return buf
		},
	}
}

func (c *challenger) sign(parts ...string) string {
	mac := hmac.New(sha256.New, c.key)
	for _, p := range parts {
		mac.Write([]byte(p))
		// A separator no field can contain keeps "ab|c" from signing the same as
		// "a|bc", which would let one field's tail be read as another's head.
		mac.Write([]byte{0})
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// issueClearance mints the cookie value proving a challenge was completed.
// It binds the client address and the kind, so a cookie lifted from one visitor
// does nothing for another and a timed clearance never answers a CAPTCHA rule.
func (c *challenger) issueClearance(ip string, kind challengeKind) string {
	exp := strconv.FormatInt(c.now().Add(clearanceTTL).Unix(), 10)
	return exp + "." + c.sign("clearance", ip, string(kind), exp)
}

func (c *challenger) clearanceValid(value, ip string, kind challengeKind) bool {
	exp, sig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || c.now().Unix() > unix {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(c.sign("clearance", ip, string(kind), exp)))
}

// cleared reports whether this request already carries a valid clearance.
func (c *challenger) cleared(r *http.Request, kind challengeKind) bool {
	if c == nil {
		return false
	}
	ck, err := r.Cookie(clearanceCookie)
	if err != nil {
		return false
	}
	return c.clearanceValid(ck.Value, clientIPString(r.RemoteAddr), kind)
}

// issueToken mints the value the challenge page carries back. `secret` is the
// expected answer for a CAPTCHA and is empty for the timed check.
//
// `issued` is signed too, which is what makes the wait real: verification
// refuses a token presented before issue + challengeWait, so lifting the token
// straight out of the page does not skip the delay.
func (c *challenger) issueToken(ip string, kind challengeKind, secret string) string {
	now := c.now().Unix()
	issued := strconv.FormatInt(now, 10)
	exp := strconv.FormatInt(now+int64(challengeTokenTTL/time.Second), 10)
	return issued + "." + exp + "." + c.sign("token", ip, string(kind), secret, issued, exp)
}

// verifyToken checks a returned challenge token. `answer` is what the visitor
// submitted; it must reproduce the signature, which is how a stateless CAPTCHA
// is checked without the gateway remembering anything.
func (c *challenger) verifyToken(token, ip string, kind challengeKind, answer string) error {
	issued, rest, ok := strings.Cut(token, ".")
	if !ok {
		return fmt.Errorf("malformed challenge")
	}
	exp, sig, ok := strings.Cut(rest, ".")
	if !ok {
		return fmt.Errorf("malformed challenge")
	}
	issuedAt, err := strconv.ParseInt(issued, 10, 64)
	if err != nil {
		return fmt.Errorf("malformed challenge")
	}
	expAt, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return fmt.Errorf("malformed challenge")
	}
	now := c.now().Unix()
	if now > expAt {
		return fmt.Errorf("challenge expired")
	}
	if !hmac.Equal([]byte(sig), []byte(c.sign("token", ip, string(kind), answer, issued, exp))) {
		return fmt.Errorf("challenge failed")
	}
	// The wait is enforced here, not by the page's timer. A client that grabs the
	// token and returns immediately is refused and must come back.
	if kind == challengeJS && now < issuedAt+int64(challengeWait/time.Second) {
		return fmt.Errorf("too early")
	}
	return nil
}

// captchaCode generates the code a visitor has to read. Digits and upper-case
// letters, minus the pairs that are hard to tell apart in a distorted glyph.
const captchaAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func (c *challenger) captchaCode() string {
	buf := c.rand(captchaLength)
	if buf == nil {
		return ""
	}
	out := make([]byte, captchaLength)
	for i, b := range buf {
		out[i] = captchaAlphabet[int(b)%len(captchaAlphabet)]
	}
	return string(out)
}

// captchaSVG draws the code as inline SVG. It is drawn rather than written as
// text so the answer is not sitting in the markup, and it is SVG rather than a
// raster image so the gateway needs no image library at all.
func (c *challenger) captchaSVG(code string) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="160" height="56" role="img" aria-label="captcha">`)
	b.WriteString(`<rect width="160" height="56" fill="#f4f5f7"/>`)
	noise := c.rand(12)
	for i := 0; i+3 < len(noise); i += 4 {
		fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#c8ccd4" stroke-width="1"/>`,
			int(noise[i])%160, int(noise[i+1])%56, int(noise[i+2])%160, int(noise[i+3])%56)
	}
	for i, ch := range code {
		angle := (i*13)%25 - 12
		x := 18 + i*34
		fmt.Fprintf(&b,
			`<text x="%d" y="40" font-family="monospace" font-size="30" fill="#2b3038" transform="rotate(%d %d 40)">%s</text>`,
			x, angle, x, html.EscapeString(string(ch)))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// safeReturnPath keeps the post-challenge redirect on this site. An unchecked
// value here would be an open redirect handed to whoever can craft a link.
func safeReturnPath(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	if strings.Contains(raw, "\r") || strings.Contains(raw, "\n") {
		return "/"
	}
	if strings.HasPrefix(raw, challengePathPrefix) {
		// Returning to the challenge itself would loop.
		return "/"
	}
	return raw
}

func (c *challenger) setClearance(w http.ResponseWriter, r *http.Request, kind challengeKind) {
	http.SetCookie(w, &http.Cookie{
		Name:     clearanceCookie,
		Value:    c.issueClearance(clientIPString(r.RemoteAddr), kind),
		Path:     "/",
		MaxAge:   int(clearanceTTL / time.Second),
		HttpOnly: false, // the timed check sets it from JavaScript
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
	})
}

// serveJSChallenge answers with the timed browser check.
func (c *challenger) serveJSChallenge(w http.ResponseWriter, r *http.Request) {
	ip := clientIPString(r.RemoteAddr)
	token := c.issueToken(ip, challengeJS, "")
	ret := html.EscapeString(safeReturnPath(r.URL.RequestURI()))
	page := fmt.Sprintf(jsChallengeHTML, int(challengeWait/time.Second),
		html.EscapeString(token), ret, int(challengeWait/time.Second))
	writeChallengePage(w, page)
}

// serveCaptcha answers with the CAPTCHA form.
func (c *challenger) serveCaptcha(w http.ResponseWriter, r *http.Request, note string) {
	ip := clientIPString(r.RemoteAddr)
	code := c.captchaCode()
	token := c.issueToken(ip, challengeCaptcha, strings.ToLower(code))
	ret := html.EscapeString(safeReturnPath(r.URL.RequestURI()))
	page := fmt.Sprintf(captchaHTML, c.captchaSVG(code), challengeVerifyPath,
		html.EscapeString(token), ret, html.EscapeString(note))
	writeChallengePage(w, page)
}

func writeChallengePage(w http.ResponseWriter, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A challenge page must never be cached: it carries a one-use token, and a
	// cached copy would hand a stale token to the next visitor.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

// handleVerify processes the CAPTCHA form post. It is served from the reserved
// path on every protected site, ahead of every other check, so the answer is not
// itself refused by the rule that demanded the challenge.
func (c *challenger) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// The form is tiny; a body larger than this is not one of ours.
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		c.serveCaptcha(w, r, "")
		return
	}
	ip := clientIPString(r.RemoteAddr)
	token := r.PostFormValue("t")
	answer := strings.ToLower(strings.TrimSpace(r.PostFormValue("a")))
	ret := safeReturnPath(r.PostFormValue("r"))

	if err := c.verifyToken(token, ip, challengeCaptcha, answer); err != nil {
		// A fresh challenge on every failure: reusing the old one would let an
		// attacker keep guessing against a single code.
		r.URL.Path = ret
		r.URL.RawQuery = ""
		c.serveCaptcha(w, r, "verification failed, please try again")
		return
	}
	c.setClearance(w, r, challengeCaptcha)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

const jsChallengeHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Checking your browser</title></head>
<body style="font-family:system-ui,sans-serif;text-align:center;padding:3rem;color:#333">
<h1 style="font-size:1.5rem;margin:0 0 .5rem">Checking your browser</h1>
<p>This takes about %d seconds.</p>
<noscript><p>JavaScript is required to continue.</p></noscript>
<script>
(function () {
  var t = "%s", r = "%s";
  setTimeout(function () {
    document.cookie = "` + clearanceCookie + `=" + encodeURIComponent(t) + "; path=/; max-age=1800";
    var f = document.createElement("form");
    f.method = "POST"; f.action = "` + challengeVerifyPath + `";
    [["t", t], ["a", ""], ["r", r]].forEach(function (p) {
      var i = document.createElement("input");
      i.type = "hidden"; i.name = p[0]; i.value = p[1];
      f.appendChild(i);
    });
    document.body.appendChild(f); f.submit();
  }, %d000);
})();
</script>
</body></html>
`

const captchaHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Verification</title></head>
<body style="font-family:system-ui,sans-serif;text-align:center;padding:3rem;color:#333">
<h1 style="font-size:1.5rem;margin:0 0 1rem">Please confirm you are not a robot</h1>
<div style="margin:0 0 1rem">%s</div>
<form method="POST" action="%s">
<input type="hidden" name="t" value="%s">
<input type="hidden" name="r" value="%s">
<input name="a" autocomplete="off" autocapitalize="off" spellcheck="false"
       style="font-size:1.1rem;padding:.4rem;width:9rem;text-align:center" aria-label="code">
<button type="submit" style="font-size:1.1rem;padding:.4rem 1rem;margin-left:.5rem">OK</button>
</form>
<p style="color:#a33">%s</p>
</body></html>
`
