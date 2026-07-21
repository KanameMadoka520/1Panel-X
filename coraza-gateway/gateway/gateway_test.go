package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const upstreamMarker = "OK-UPSTREAM-REACHED"

func recordingUpstream(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, upstreamMarker)
	})
}

func buildGateway(t *testing.T, mode Mode, bodyLimit int, reached *bool) *Handler {
	t.Helper()
	eng, err := NewEngine(mode, bodyLimit)
	if err != nil {
		t.Fatalf("engine build failed: %v", err)
	}
	return NewHandler(eng, recordingUpstream(reached), mode)
}

func serve(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// Encoded attack payloads (kept out of the URL parser's way).
const (
	sqliTarget      = "/?id=1%27%20OR%20%271%27%3D%271%20UNION%20SELECT%20password%20FROM%20users--"
	xssTarget       = "/?q=%3Cscript%3Ealert(1)%3C%2Fscript%3E"
	traversalTarget = "/?file=..%2F..%2F..%2F..%2F..%2Fetc%2Fpasswd"
)

func TestCleanRequestReachesUpstream(t *testing.T) {
	reached := false
	h := buildGateway(t, ModeBlock, 1<<20, &reached)
	rr := serve(h, "GET", "/products?page=2&sort=name", "")
	if rr.Code != http.StatusOK || !reached {
		t.Fatalf("clean request should reach upstream 200: code=%d reached=%v", rr.Code, reached)
	}
	if !strings.Contains(rr.Body.String(), upstreamMarker) {
		t.Fatalf("expected upstream body, got %q", rr.Body.String())
	}
}

func TestSQLiBlockedInBlockMode(t *testing.T) {
	reached := false
	h := buildGateway(t, ModeBlock, 1<<20, &reached)
	rr := serve(h, "GET", sqliTarget, "")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("SQLi should be blocked 403, got %d", rr.Code)
	}
	if reached {
		t.Fatal("upstream must NOT be reached when blocked")
	}
	if strings.Contains(rr.Body.String(), upstreamMarker) {
		t.Fatal("block response must not contain upstream body (W7 leak)")
	}
	if !strings.Contains(rr.Body.String(), "blocked") {
		t.Fatalf("block page expected, got %q", rr.Body.String())
	}
}

func TestXSSBlockedInBlockMode(t *testing.T) {
	reached := false
	h := buildGateway(t, ModeBlock, 1<<20, &reached)
	rr := serve(h, "GET", xssTarget, "")
	if rr.Code != http.StatusForbidden || reached {
		t.Fatalf("XSS should be blocked: code=%d reached=%v", rr.Code, reached)
	}
}

func TestPathTraversalBlockedInBlockMode(t *testing.T) {
	reached := false
	h := buildGateway(t, ModeBlock, 1<<20, &reached)
	rr := serve(h, "GET", traversalTarget, "")
	if rr.Code != http.StatusForbidden || reached {
		t.Fatalf("path traversal should be blocked: code=%d reached=%v", rr.Code, reached)
	}
}

// The defining difference of detection mode: the same attack is logged but
// passes through to the upstream (explicitly-labelled fail-open).
func TestDetectionModeLetsAttackThrough(t *testing.T) {
	reached := false
	h := buildGateway(t, ModeDetection, 1<<20, &reached)
	rr := serve(h, "GET", sqliTarget, "")
	if rr.Code != http.StatusOK || !reached {
		t.Fatalf("detection mode must pass the attack to upstream: code=%d reached=%v", rr.Code, reached)
	}
}

// W9: a broken candidate ruleset is rejected and the running engine keeps
// blocking — never a half-loaded or downed WAF.
func TestReloadRejectsBadRulesetAndKeepsBlocking(t *testing.T) {
	reached := false
	eng, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	if err := eng.reloadRaw("SecRequestBodyLimit not-a-number\n"); err == nil {
		t.Fatal("a malformed ruleset must fail to compile")
	}
	h := NewHandler(eng, recordingUpstream(&reached), ModeBlock)
	rr := serve(h, "GET", sqliTarget, "")
	if rr.Code != http.StatusForbidden || reached {
		t.Fatalf("engine must still block after a rejected reload: code=%d reached=%v", rr.Code, reached)
	}
}

func TestReloadGoodSwaps(t *testing.T) {
	eng, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	if err := eng.Reload(ModeBlock, 1<<20); err != nil {
		t.Fatalf("valid reload should succeed: %v", err)
	}
}

// W3: an over-limit request body is rejected, not buffered unbounded.
func TestOversizeBodyRejected(t *testing.T) {
	reached := false
	h := buildGateway(t, ModeBlock, 512, &reached)
	rr := serve(h, "POST", "/upload", strings.Repeat("a", 4096))
	if rr.Code < 400 || reached {
		t.Fatalf("oversize body should be rejected pre-upstream: code=%d reached=%v", rr.Code, reached)
	}
}

func TestChunkedOversizeBodyRejected(t *testing.T) {
	reached := false
	h := buildGateway(t, ModeBlock, 512, &reached)
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("a", 4096)))
	req.ContentLength = -1
	req.TransferEncoding = []string{"chunked"}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code < 400 || reached {
		t.Fatalf("chunked oversize body should be rejected pre-upstream: code=%d reached=%v", rr.Code, reached)
	}
}

// W1 recover policy, exercised directly across the four (mode, ran) combinations.
func TestOnPanicPolicy(t *testing.T) {
	newH := func(mode Mode, reached *bool) *Handler {
		return &Handler{engine: nil, upstream: recordingUpstream(reached), mode: mode}
	}
	req := httptest.NewRequest("GET", "/x", nil)

	// block + not-yet-run → fail closed 403, generic page, upstream untouched.
	{
		reached := false
		rr := httptest.NewRecorder()
		newH(ModeBlock, &reached).onPanic(rr, req, false)
		if rr.Code != http.StatusForbidden || reached || !strings.Contains(rr.Body.String(), "blocked") {
			t.Fatalf("block/not-run must fail closed: code=%d reached=%v", rr.Code, reached)
		}
	}
	// detection + not-yet-run → fail OPEN (proxy through).
	{
		reached := false
		rr := httptest.NewRecorder()
		newH(ModeDetection, &reached).onPanic(rr, req, false)
		if !reached || !strings.Contains(rr.Body.String(), upstreamMarker) {
			t.Fatalf("detection/not-run must fail open to upstream: reached=%v body=%q", reached, rr.Body.String())
		}
	}
	// detection + already-run → cannot resume, fail closed.
	{
		reached := false
		rr := httptest.NewRecorder()
		newH(ModeDetection, &reached).onPanic(rr, req, true)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("detection/already-run must fail closed: code=%d", rr.Code)
		}
	}
}

// A panic raised downstream of a passing request is recovered (never a 500 leak
// or hung connection) and, in block mode, answered as a block.
func TestPanicDuringUpstreamRecovered(t *testing.T) {
	eng, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	panicUpstream := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	h := NewHandler(eng, panicUpstream, ModeBlock)
	rr := serve(h, "GET", "/clean/path", "")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("panic must be recovered to a block, got %d", rr.Code)
	}
}
