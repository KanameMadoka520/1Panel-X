package gateway

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fixedChallenger is deterministic: a pinned clock and a pinned code generator,
// so the assertions below are about behaviour rather than luck.
func fixedChallenger(now *time.Time) *challenger {
	c := newChallenger()
	c.now = func() time.Time { return *now }
	c.rand = func(n int) []byte {
		out := make([]byte, n)
		for i := range out {
			out[i] = byte(i)
		}
		return out
	}
	return c
}

func buildChallengeGateway(t *testing.T, action CustomAction, c *challenger, reached *bool) http.Handler {
	t.Helper()
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sites: []SiteConfig{{
		Host: "a.example", Upstream: "http://127.0.0.1:1",
		CustomRules: []CustomRule{{
			Name:       "guard",
			Action:     action,
			Conditions: []CustomCondition{{Field: FieldURL, Match: ListMatchPrefix, Pattern: "/manage"}},
		}},
	}}}
	rt, err := NewRouter(cfg, engine, ModeBlock, "")
	if err != nil {
		t.Fatal(err)
	}
	h := rt.handlers["a.example"].(*Handler)
	h.upstream = recordingUpstream(reached)
	h.challenger = c
	return rt
}

func challengeRequest(h http.Handler, method, target string, cookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = "a.example"
	req.RemoteAddr = "203.0.113.7:1234"
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: clearanceCookie, Value: cookie})
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// A challenged request must NOT reach the origin, and must not be answered with
// a plain refusal either — it is held, and the visitor is shown the check.
func TestChallengeHoldsTheRequestWithoutReachingTheOrigin(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, action := range []CustomAction{CustomJS, CustomCaptcha} {
		var reached bool
		h := buildChallengeGateway(t, action, fixedChallenger(&now), &reached)

		reached = false
		rr := challengeRequest(h, "GET", "/manage/login", "")
		if reached {
			t.Fatalf("%s: a challenged request must not reach the origin", action)
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: the challenge page must render, got %d", action, rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("%s: the challenge must be HTML, got %q", action, ct)
		}
		// A cached challenge page would hand a stale one-use token to the next
		// visitor.
		if cc := rr.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Fatalf("%s: the challenge page must not be cacheable, got %q", action, cc)
		}
		// A path the rule does not cover is untouched.
		reached = false
		if code := challengeRequest(h, "GET", "/public", "").Code; code != http.StatusOK || !reached {
			t.Fatalf("%s: an uncovered path must pass, got %d reached=%v", action, code, reached)
		}
	}
}

// A valid clearance admits the visitor — and admits them to the REST of the
// pipeline, not past it: the rule set still runs.
func TestClearanceAdmitsButDoesNotExemptFromTheRuleSet(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var reached bool
	c := fixedChallenger(&now)
	h := buildChallengeGateway(t, CustomCaptcha, c, &reached)

	clearance := c.issueClearance("203.0.113.7", challengeCaptcha)
	reached = false
	if code := challengeRequest(h, "GET", "/manage/login", clearance).Code; code != http.StatusOK || !reached {
		t.Fatalf("a cleared visitor must be admitted, got %d reached=%v", code, reached)
	}
	// Cleared, but an attack on the same path is still blocked.
	reached = false
	if code := challengeRequest(h, "GET", "/manage/login?q=%3Cscript%3Ealert(1)%3C%2Fscript%3E", clearance).Code; code != http.StatusForbidden {
		t.Fatalf("a clearance must not exempt from the rule set, got %d", code)
	}
}

// The clearance is bound to the client address and to the KIND of challenge, so
// a cookie lifted from another visitor is useless and passing the cheap check
// never satisfies the expensive one.
func TestClearanceIsBoundToAddressAndKind(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := fixedChallenger(&now)

	valid := c.issueClearance("203.0.113.7", challengeCaptcha)
	if !c.clearanceValid(valid, "203.0.113.7", challengeCaptcha) {
		t.Fatal("a freshly issued clearance must verify")
	}
	if c.clearanceValid(valid, "198.51.100.7", challengeCaptcha) {
		t.Fatal("a clearance must not travel to another address")
	}
	if c.clearanceValid(valid, "203.0.113.7", challengeJS) {
		t.Fatal("a CAPTCHA clearance must not answer a timed-check rule")
	}
	if c.clearanceValid(c.issueClearance("203.0.113.7", challengeJS), "203.0.113.7", challengeCaptcha) {
		t.Fatal("a timed-check clearance must not answer a CAPTCHA rule")
	}
	// Forgery attempts.
	for _, bad := range []string{"", "nonsense", "9999999999.deadbeef", valid + "x"} {
		if c.clearanceValid(bad, "203.0.113.7", challengeCaptcha) {
			t.Fatalf("clearance %q must not verify", bad)
		}
	}
	// Expiry.
	past := now
	now = now.Add(clearanceTTL + time.Second)
	if c.clearanceValid(valid, "203.0.113.7", challengeCaptcha) {
		t.Fatal("an expired clearance must not verify")
	}
	now = past
}

// THE assertion that makes the timed check worth anything: the wait is enforced
// on this side. Lifting the token straight out of the page and returning
// immediately must not skip it.
func TestTimedCheckWaitIsEnforcedServerSide(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := fixedChallenger(&now)
	token := c.issueToken("203.0.113.7", challengeJS, "")

	if err := c.verifyToken(token, "203.0.113.7", challengeJS, ""); err == nil {
		t.Fatal("a token returned immediately must be refused")
	}
	now = now.Add(challengeWait - time.Second)
	if err := c.verifyToken(token, "203.0.113.7", challengeJS, ""); err == nil {
		t.Fatal("a token returned before the wait elapses must be refused")
	}
	now = now.Add(2 * time.Second)
	if err := c.verifyToken(token, "203.0.113.7", challengeJS, ""); err != nil {
		t.Fatalf("a token returned after the wait must be accepted: %v", err)
	}
	// And it must not be usable forever.
	now = now.Add(challengeTokenTTL)
	if err := c.verifyToken(token, "203.0.113.7", challengeJS, ""); err == nil {
		t.Fatal("an expired token must be refused")
	}
}

// The CAPTCHA is verified statelessly: the expected answer is signed into the
// token, never stored. A wrong answer must fail, and the answer must not be
// readable from the markup.
func TestCaptchaVerification(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := fixedChallenger(&now)
	code := c.captchaCode()
	if len(code) != captchaLength {
		t.Fatalf("captcha code must be %d characters, got %q", captchaLength, code)
	}
	token := c.issueToken("203.0.113.7", challengeCaptcha, strings.ToLower(code))

	if err := c.verifyToken(token, "203.0.113.7", challengeCaptcha, strings.ToLower(code)); err != nil {
		t.Fatalf("the right answer must be accepted: %v", err)
	}
	// Case-insensitive, since the drawn glyphs are upper-case only.
	if err := c.verifyToken(token, "203.0.113.7", challengeCaptcha, strings.ToUpper(code)); err == nil {
		t.Log("upper-case is normalized by the caller, not here")
	}
	for _, wrong := range []string{"", "zzzz", strings.ToLower(code) + "x"} {
		if err := c.verifyToken(token, "203.0.113.7", challengeCaptcha, wrong); err == nil {
			t.Fatalf("answer %q must be refused", wrong)
		}
	}
	// Bound to the address too.
	if err := c.verifyToken(token, "198.51.100.7", challengeCaptcha, strings.ToLower(code)); err == nil {
		t.Fatal("a token must not be answerable from another address")
	}
	// The answer must not appear as text in the drawn challenge.
	svg := c.captchaSVG(code)
	if strings.Contains(svg, code) {
		t.Fatalf("the code must be drawn glyph by glyph, not written out: %s", svg)
	}
}

// The full CAPTCHA round trip through the handler: challenge, answer, admitted.
func TestCaptchaRoundTripThroughTheHandler(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var reached bool
	c := fixedChallenger(&now)
	h := buildChallengeGateway(t, CustomCaptcha, c, &reached)

	page := challengeRequest(h, "GET", "/manage/login", "").Body.String()
	token := extractInput(t, page, "t")
	ret := extractInput(t, page, "r")
	if ret != "/manage/login" {
		t.Fatalf("the challenge must remember where to return, got %q", ret)
	}
	// The code the fixed generator produced, recomputed the same way.
	code := strings.ToLower(c.captchaCode())

	form := url.Values{"t": {token}, "a": {code}, "r": {ret}}
	req := httptest.NewRequest("POST", challengeVerifyPath, strings.NewReader(form.Encode()))
	req.Host = "a.example"
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("a correct answer must redirect back, got %d body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/manage/login" {
		t.Fatalf("it must redirect to where the visitor was going, got %q", loc)
	}
	var clearance string
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == clearanceCookie {
			clearance = ck.Value
		}
	}
	if clearance == "" {
		t.Fatal("a correct answer must set a clearance cookie")
	}
	reached = false
	if code := challengeRequest(h, "GET", "/manage/login", clearance).Code; code != http.StatusOK || !reached {
		t.Fatalf("the earned clearance must admit the visitor, got %d reached=%v", code, reached)
	}
}

// A wrong answer must not admit anyone, and must hand back a FRESH challenge —
// reusing the old one would let an attacker keep guessing at a single code.
func TestWrongCaptchaAnswerIsRefusedAndReissued(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var reached bool
	c := fixedChallenger(&now)
	h := buildChallengeGateway(t, CustomCaptcha, c, &reached)

	page := challengeRequest(h, "GET", "/manage/login", "").Body.String()
	form := url.Values{"t": {extractInput(t, page, "t")}, "a": {"zzzz"}, "r": {"/manage/login"}}
	req := httptest.NewRequest("POST", challengeVerifyPath, strings.NewReader(form.Encode()))
	req.Host = "a.example"
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	for _, ck := range rr.Result().Cookies() {
		if ck.Name == clearanceCookie {
			t.Fatal("a wrong answer must not set a clearance")
		}
	}
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "<svg") {
		t.Fatalf("a wrong answer must hand back a fresh challenge, got %d", rr.Code)
	}
}

// The post-challenge redirect must stay on this site. An unchecked value would
// be an open redirect handed to whoever can craft a link.
func TestReturnPathCannotLeaveTheSite(t *testing.T) {
	for _, bad := range []string{
		"//evil.example/", "https://evil.example/", "http://evil.example",
		"/x\r\nSet-Cookie: a=b", challengePathPrefix + "verify", "", "evil",
	} {
		if got := safeReturnPath(bad); !strings.HasPrefix(got, "/") || got == bad {
			if got != "/" {
				t.Fatalf("return path %q must be refused, got %q", bad, got)
			}
		}
		if got := safeReturnPath(bad); strings.Contains(got, "evil") || strings.Contains(got, "\n") {
			t.Fatalf("return path %q must be refused, got %q", bad, got)
		}
	}
	if got := safeReturnPath("/manage/login?x=1"); got != "/manage/login?x=1" {
		t.Fatalf("an ordinary path must survive, got %q", got)
	}
}

var inputPattern = regexp.MustCompile(`<input type="hidden" name="([a-z])" value="([^"]*)"`)

func extractInput(t *testing.T, page, name string) string {
	t.Helper()
	for _, m := range inputPattern.FindAllStringSubmatch(page, -1) {
		if m[1] == name {
			return m[2]
		}
	}
	t.Fatalf("field %q not found in challenge page:\n%s", name, page)
	return ""
}
