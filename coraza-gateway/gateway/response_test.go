package gateway

import (
	"net/http"
	"testing"
)

func TestStripFingerprintHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Server", "nginx/1.25.3")
	h.Set("X-Powered-By", "PHP/8.1.2")
	h.Set("X-AspNet-Version", "4.0.30319")
	h.Set("Content-Type", "text/html")
	h.Set("Cache-Control", "no-cache")

	StripFingerprintHeaders(h)

	for _, banner := range []string{"Server", "X-Powered-By", "X-AspNet-Version"} {
		if h.Get(banner) != "" {
			t.Errorf("fingerprint header %q should be stripped", banner)
		}
	}
	if h.Get("Content-Type") != "text/html" || h.Get("Cache-Control") != "no-cache" {
		t.Fatalf("non-fingerprint headers must be preserved: %+v", h)
	}
}
