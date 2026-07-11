package middleware

import "testing"

func TestIsPasswordExpiredPublicPathAllowsOpenEnhancementSubset(t *testing.T) {
	if !isPasswordExpiredPublicPath("/api/v2/core/settings/enhancements/public") {
		t.Fatal("expected public enhancement settings to be available before login")
	}
}

func TestIsPasswordExpiredPublicPathProtectsFullEnhancementSettings(t *testing.T) {
	protected := []string{
		"/api/v2/core/settings/enhancements/search",
		"/api/v2/core/settings/enhancements/update",
	}
	for _, requestPath := range protected {
		if isPasswordExpiredPublicPath(requestPath) {
			t.Fatalf("expected %s to require an authenticated session", requestPath)
		}
	}
}
