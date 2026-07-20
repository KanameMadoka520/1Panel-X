package gateway

import (
	"encoding/json"
	"net/http"
)

// Health reports that the listener and compiled Coraza engine are ready. The
// management path is intentionally served by the same loopback/container-only
// listener; nginx never exposes it as a public website route.
type Health struct {
	Status     string `json:"status"`
	Sites      int    `json:"sites"`
	Mode       Mode   `json:"mode"`
	Generation string `json:"generation,omitempty"`
}

func WithHealth(handler http.Handler, sites int, mode Mode, generation string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" && isLoopbackHealthHost(normalizeHost(r.Host)) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(Health{Status: "ready", Sites: sites, Mode: mode, Generation: generation})
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func isLoopbackHealthHost(host string) bool {
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
