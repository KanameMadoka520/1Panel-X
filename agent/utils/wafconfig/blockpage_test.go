package wafconfig

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeBlockPage(t *testing.T) {
	// A 5xx would blame the origin for a decision the WAF made, and a 3xx would
	// turn a refusal into a redirect the operator would then have to secure.
	for _, bad := range []int{500, 502, 302, 301, 418} {
		if _, err := NormalizeBlockPage(BlockPage{Status: bad}); err == nil {
			t.Fatalf("status %d must be refused", bad)
		}
	}
	for _, ok := range []int{0, 200, 403, 404} {
		if _, err := NormalizeBlockPage(BlockPage{Status: ok}); err != nil {
			t.Fatalf("status %d must be accepted: %v", ok, err)
		}
	}
	if _, err := NormalizeBlockPage(BlockPage{HTML: strings.Repeat("a", MaxBlockPageBytes+1)}); err == nil {
		t.Fatal("an oversized block page must be refused")
	}
	// Whitespace-only HTML is the built-in page, not a blank page.
	got, err := NormalizeBlockPage(BlockPage{HTML: "   \n  "})
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Fatalf("a whitespace-only page must mean the built-in one, got %+v", got)
	}
}

func TestNormalizeLogSettings(t *testing.T) {
	got, err := NormalizeLogSettings(LogSettings{
		RetentionDays: 14,
		ExcludedKinds: []string{" ratelimit ", "acl-deny", "ratelimit", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.ExcludedKinds, []string{"acl-deny", "ratelimit"}) {
		t.Fatalf("kinds must be trimmed, de-duplicated and sorted: %#v", got.ExcludedKinds)
	}
	// A misspelled kind is refused rather than ignored: accepting it silently
	// would leave the operator believing they had switched something off.
	if _, err := NormalizeLogSettings(LogSettings{ExcludedKinds: []string{"ratelimits"}}); err == nil {
		t.Fatal("a misspelled record kind must be refused")
	}
	if _, err := NormalizeLogSettings(LogSettings{RetentionDays: MaxRetentionDays + 1}); err == nil {
		t.Fatal("an absurd retention must be refused")
	}
	if _, err := NormalizeLogSettings(LogSettings{RetentionDays: -1}); err == nil {
		t.Fatal("a negative retention must be refused")
	}
}

func TestRetentionDaysOr(t *testing.T) {
	var absent *LogSettings
	if got := absent.RetentionDaysOr(7); got != 7 {
		t.Fatalf("an absent policy must fall back, got %d", got)
	}
	if got := absent.RetentionDaysOr(0); got != DefaultRetentionDays {
		t.Fatalf("no fallback either must use the default, got %d", got)
	}
	stored := &LogSettings{RetentionDays: 90}
	if got := stored.RetentionDaysOr(7); got != 90 {
		t.Fatalf("a stored policy must win, got %d", got)
	}
}

// The data plane is given only what it can act on. Retention must never be sent:
// the panel owns the database the records end up in, and letting the gateway
// delete them would put deletion on the side that cannot know what has already
// been ingested.
func TestRetentionIsNotSentToTheDataPlane(t *testing.T) {
	cfg, err := BuildWithOptions(nil, BuildOptions{
		Log: &LogSettings{RetentionDays: 14, ExcludedKinds: []string{"ratelimit"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log == nil || !reflect.DeepEqual(cfg.Log.ExcludedKinds, []string{"ratelimit"}) {
		t.Fatalf("exclusions must reach the data plane: %+v", cfg.Log)
	}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "retentionDays") {
		t.Fatalf("retention must not appear in the emitted config:\n%s", data)
	}
	// Retention alone changes nothing the gateway sees, so it must not churn the
	// generation and force a needless reload.
	other, err := BuildWithOptions(nil, BuildOptions{
		Log: &LogSettings{RetentionDays: 999, ExcludedKinds: []string{"ratelimit"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if other.Generation != cfg.Generation {
		t.Fatal("a retention-only change must not churn the config generation")
	}
}

func TestBuildRejectsInvalidBlockPageAndLogSettings(t *testing.T) {
	if _, err := BuildWithOptions(nil, BuildOptions{BlockPage: &BlockPage{Status: 500}}); err == nil {
		t.Fatal("an invalid block page must fail config generation")
	}
	if _, err := BuildWithOptions(nil, BuildOptions{Log: &LogSettings{ExcludedKinds: []string{"nope"}}}); err == nil {
		t.Fatal("an unknown record kind must fail config generation")
	}
}
