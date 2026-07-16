package gateway

import "net/http"

// fingerprintHeaders are upstream response headers that disclose backend software
// / version and must not reach the public client through the WAF (W7).
var fingerprintHeaders = []string{
	"Server",
	"X-Powered-By",
	"X-AspNet-Version",
	"X-AspNetMvc-Version",
	"X-Generator",
	"X-Runtime",
	"X-Version",
}

// StripFingerprintHeaders removes upstream banner/version headers from a proxied
// response so the WAF does not leak backend fingerprints (W7).
func StripFingerprintHeaders(h http.Header) {
	for _, k := range fingerprintHeaders {
		h.Del(k)
	}
}
