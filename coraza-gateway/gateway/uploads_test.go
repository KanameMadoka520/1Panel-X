package gateway

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveUpload posts one multipart file part under the given client-supplied
// filename, which is what the upload-extension rule inspects.
func serveUpload(h http.Handler, filename string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", filename)
	_, _ = part.Write([]byte("harmless content"))
	_ = w.Close()

	req := httptest.NewRequest("POST", "/upload", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// uncoveredExts are extensions the bundled rule set does NOT already refuse
// (verified by TestUncoveredExtensionsAreNotAlreadyBlocked below). The
// enforcement tests deliberately use these rather than `php` or `jsp`: the rule
// set blocks those uploads on its own, so a test written against them would pass
// even if our own rule did nothing at all.
var uncoveredExts = []string{"iso", "apk", "img"}

// The load-bearing assertion: a banned extension is actually refused and never
// reaches the origin, while an ordinary upload still goes through. A list that
// is stored and displayed but not enforced is precisely the failure this guards.
func TestBannedUploadExtensionIsRefused(t *testing.T) {
	var reached bool
	h := buildPolicyGateway(t, RulePolicy{BannedUploadExts: uncoveredExts}, &reached)

	for _, name := range []string{"release.iso", "Release.ISO", "app.apk", "disk.img"} {
		reached = false
		if code := serveUpload(h, name).Code; code != http.StatusForbidden {
			t.Fatalf("upload %q must be refused, got %d", name, code)
		}
		if reached {
			t.Fatalf("upload %q must not reach the origin", name)
		}
	}

	reached = false
	if code := serveUpload(h, "holiday.jpg").Code; code != http.StatusOK || !reached {
		t.Fatalf("an ordinary upload must pass, got %d reached=%v", code, reached)
	}
}

// This is what makes TestBannedUploadExtensionIsRefused meaningful: it proves
// the extensions used there are not already refused by the rule set, so a pass
// there can only come from our own rule.
func TestUncoveredExtensionsAreNotAlreadyBlocked(t *testing.T) {
	var reached bool
	h := buildPolicyGateway(t, RulePolicy{}, &reached)
	for _, ext := range uncoveredExts {
		reached = false
		if code := serveUpload(h, "sample."+ext).Code; code != http.StatusOK || !reached {
			t.Fatalf("extension %q is already refused by the rule set, so it cannot prove our rule works (got %d)", ext, code)
		}
	}
}

// A double extension is the classic way to smuggle a banned extension past a
// check that only looks at the final component.
func TestBannedUploadExtensionResistsEvasion(t *testing.T) {
	var reached bool
	h := buildPolicyGateway(t, RulePolicy{BannedUploadExts: []string{"iso"}}, &reached)

	for _, name := range []string{"release.iso.jpg", "release.IsO.png", "release.iso."} {
		reached = false
		if code := serveUpload(h, name).Code; code != http.StatusForbidden {
			t.Fatalf("evasive upload %q must be refused, got %d", name, code)
		}
	}
	// A filename carrying a NUL is refused earlier, by the multipart parser
	// itself, before any rule sees it. That is still a fail-closed refusal, so
	// the assertion is "not admitted" rather than a specific status: pinning 403
	// here would be asserting which layer refuses it, which is not the contract.
	reached = false
	if rr := serveUpload(h, "release.iso\x00.jpg"); rr.Code == http.StatusOK || reached {
		t.Fatalf("a NUL-carrying upload name must not be admitted, got %d reached=%v", rr.Code, reached)
	}
	// The extension must be a whole component: a name that merely contains the
	// letters must not be refused, or the control becomes unusable.
	reached = false
	if code := serveUpload(h, "isonotes.txt").Code; code != http.StatusOK || !reached {
		t.Fatalf("a name that only contains the letters must pass, got %d reached=%v", code, reached)
	}
}

// An empty list must emit no rule at all, so a site that bans nothing pays
// nothing and nothing is refused by this control.
func TestNoBannedExtensionsEmitsNoUploadRule(t *testing.T) {
	if got := uploadDenyDirective(""); got != "" {
		t.Fatalf("an empty list must emit no directive, got %q", got)
	}
	if got := directivesFor(enginePolicy{Mode: ModeBlock}, ""); strings.Contains(got, "FILES") {
		t.Fatalf("no list configured must emit no upload rule:\n%s", got)
	}
}

func TestExtensionNormalizationRejectsDirectiveInjection(t *testing.T) {
	// The list is interpolated into a SecRule regular expression inside a quoted
	// directive. A quote, a newline or a regex metacharacter could terminate the
	// directive and append another one.
	bad := []string{
		`php" "id:1,phase:1,pass" SecRuleEngine Off "`,
		"php\nSecRuleEngine Off",
		"php|jsp",
		"p.p",
		"php)(",
		strings.Repeat("a", maxExtensionLength+1),
	}
	for _, e := range bad {
		if _, err := normalizeExtensions([]string{e}); err == nil {
			t.Fatalf("extension %q must be rejected", e)
		}
	}
	got, err := normalizeExtensions([]string{" .PHP ", "jsp", "php", ""})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "jsp php" {
		t.Fatalf("extensions must be lower-cased, dot-stripped, de-duplicated and sorted, got %v", got)
	}
	if _, err := normalizeExtensions(make([]string, maxBannedExtensions+1)); err == nil {
		t.Fatal("an oversized extension list must be rejected")
	}
}

func TestParseConfigRejectsBadUploadExtensions(t *testing.T) {
	body := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rules":{"bannedUploadExts":["php\" \"id:1"]}}]}`
	if _, err := ParseConfig([]byte(body)); err == nil {
		t.Fatal("an extension that could inject directives must fail the config load")
	}
	ok := `{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1","rules":{"bannedUploadExts":["PHP","jsp"]}}]}`
	cfg, err := ParseConfig([]byte(ok))
	if err != nil {
		t.Fatalf("a valid extension list was rejected: %v", err)
	}
	p, err := cfg.Sites[0].enginePolicy(ModeDetection)
	if err != nil {
		t.Fatal(err)
	}
	if p.BannedUploadExts != "jsp php" {
		t.Fatalf("extension list did not round-trip: %q", p.BannedUploadExts)
	}
}

// Detection mode must observe the upload without refusing it, exactly like every
// other detection: a mode labelled "detection" that silently blocks would be a
// lie in the other direction.
func TestBannedUploadIsObservedNotBlockedInDetectionMode(t *testing.T) {
	var reached bool
	site := SiteConfig{
		Host: "a.example", Upstream: "http://127.0.0.1:1", Mode: ModeDetection,
		Rules: &RulePolicy{BannedUploadExts: []string{"iso"}},
	}
	p, err := site.enginePolicy(ModeDetection)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := newEngine(p, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(engine, recordingUpstream(&reached), ModeDetection)
	if code := serveUpload(h, "release.iso").Code; code != http.StatusOK || !reached {
		t.Fatalf("detection mode must not refuse the upload, got %d reached=%v", code, reached)
	}
}
