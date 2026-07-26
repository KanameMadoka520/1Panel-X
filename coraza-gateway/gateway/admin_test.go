package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newAdminFixture(t *testing.T, token string) (http.Handler, *Enforcer) {
	t.Helper()
	journal, _ := newJournalFixture(t)
	enforcer := NewEnforcer(journal)
	site := siteRef{WebsiteID: 1, Host: "a.example"}
	cfg := RateLimitConfig{Kind: RateLimitAccess, PeriodSec: 60, Threshold: 1, BanSec: 600}
	enforcer.count(site, cfg, "203.0.113.50", "/")

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	return WithAdmin(inner, enforcer, token), enforcer
}

func adminRequest(h http.Handler, method, target, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		req.Header.Set(AdminTokenHeader, token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdminStateListsLiveBans(t *testing.T) {
	h, _ := newAdminFixture(t, "s3cret")

	rec := adminRequest(h, "GET", "http://127.0.0.1/admin/state", "s3cret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("state should be readable with the token, got %d", rec.Code)
	}
	var state AdminState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("state is not valid JSON: %v", err)
	}
	if len(state.Bans) != 1 || state.Bans[0].IP != "203.0.113.50" {
		t.Fatalf("the live ban must be reported: %+v", state.Bans)
	}
	if state.Bans[0].ExpiresAt.Before(time.Now()) {
		t.Fatal("a live ban must carry a future expiry so the panel can derive its state")
	}
	if state.TrackedCounters < 1 {
		t.Fatalf("counter usage must be reported, got %d", state.TrackedCounters)
	}
}

func TestAdminRejectsMissingAndWrongToken(t *testing.T) {
	h, _ := newAdminFixture(t, "s3cret")

	// The loopback host check alone must not be enough: under network_mode host
	// any local process satisfies it.
	if code := adminRequest(h, "GET", "http://127.0.0.1/admin/state", "", "").Code; code != http.StatusForbidden {
		t.Fatalf("a request with no token must be refused, got %d", code)
	}
	if code := adminRequest(h, "GET", "http://127.0.0.1/admin/state", "wrong", "").Code; code != http.StatusForbidden {
		t.Fatalf("a wrong token must be refused, got %d", code)
	}
}

func TestAdminPathFromASiteHostFallsThroughToTheRouter(t *testing.T) {
	h, _ := newAdminFixture(t, "s3cret")
	// A protected website may legitimately serve /admin/, and it must keep
	// working: only loopback-addressed requests are treated as management calls.
	rec := adminRequest(h, "GET", "http://a.example/admin/state", "s3cret", "")
	if rec.Code != http.StatusTeapot {
		t.Fatalf("a site-addressed /admin/ path must reach the site, got %d", rec.Code)
	}
}

func TestAdminReleaseLiftsBan(t *testing.T) {
	h, enforcer := newAdminFixture(t, "s3cret")

	rec := adminRequest(h, "POST", "http://127.0.0.1/admin/bans/release", "s3cret", `{"ip":"203.0.113.50"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("release should succeed, got %d", rec.Code)
	}
	var out releaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || !out.Released {
		t.Fatalf("release should report success: %v %+v", err, out)
	}
	if _, banned := enforcer.Banned("203.0.113.50"); banned {
		t.Fatal("the ban must actually be gone, not just reported as released")
	}

	// Releasing an address that is not banned reports honestly instead of lying.
	rec = adminRequest(h, "POST", "http://127.0.0.1/admin/bans/release", "s3cret", `{"ip":"198.51.100.1"}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Released {
		t.Fatalf("releasing an unbanned address must report released=false: %v %+v", err, out)
	}
}

func TestAdminRejectsBadRequests(t *testing.T) {
	h, _ := newAdminFixture(t, "s3cret")

	if code := adminRequest(h, "POST", "http://127.0.0.1/admin/state", "s3cret", "").Code; code != http.StatusMethodNotAllowed {
		t.Fatalf("state is read-only, got %d", code)
	}
	if code := adminRequest(h, "GET", "http://127.0.0.1/admin/bans/release", "s3cret", "").Code; code != http.StatusMethodNotAllowed {
		t.Fatalf("release must be a POST, got %d", code)
	}
	if code := adminRequest(h, "POST", "http://127.0.0.1/admin/bans/release", "s3cret", "{").Code; code != http.StatusBadRequest {
		t.Fatalf("malformed JSON must be rejected, got %d", code)
	}
	if code := adminRequest(h, "POST", "http://127.0.0.1/admin/bans/release", "s3cret", `{"ip":"  "}`).Code; code != http.StatusBadRequest {
		t.Fatalf("an empty address must be rejected, got %d", code)
	}
	if code := adminRequest(h, "GET", "http://127.0.0.1/admin/nope", "s3cret", "").Code; code != http.StatusNotFound {
		t.Fatalf("an unknown management path must 404, got %d", code)
	}
}

func TestAdminDisabledWithoutAToken(t *testing.T) {
	h, _ := newAdminFixture(t, "")
	// No token configured means no management API at all — an ungated mutation
	// endpoint would be worse than no capability.
	if code := adminRequest(h, "GET", "http://127.0.0.1/admin/state", "", "").Code; code != http.StatusNotFound {
		t.Fatalf("the management API must be absent without a token, got %d", code)
	}
	// Ordinary traffic is unaffected.
	if code := adminRequest(h, "GET", "http://a.example/", "", "").Code; code != http.StatusTeapot {
		t.Fatalf("site traffic must still be served, got %d", code)
	}
}

func TestReadAdminTokenDegradesGracefully(t *testing.T) {
	if got := ReadAdminToken(""); got != "" {
		t.Fatal("an empty path must disable the management API")
	}
	if got := ReadAdminToken(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Fatal("a missing token file must disable the API rather than fail startup")
	}
	path := filepath.Join(t.TempDir(), "admin.token")
	if err := os.WriteFile(path, []byte("  tok3n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadAdminToken(path); got != "tok3n" {
		t.Fatalf("token should be trimmed, got %q", got)
	}
}
