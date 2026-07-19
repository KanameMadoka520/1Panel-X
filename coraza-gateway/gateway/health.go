package gateway

import (
	"encoding/json"
	"net/http"
)

// Health reports that the listener and compiled Coraza engine are ready. The
// management path is intentionally served by the same loopback/container-only
// listener; nginx never exposes it as a public website route.
type Health struct {
	Status string `json:"status"`
	Sites  int    `json:"sites"`
	Mode   Mode   `json:"mode"`
}

func WithHealth(handler http.Handler, sites int, mode Mode) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Health{Status: "ready", Sites: sites, Mode: mode})
			return
		}
		handler.ServeHTTP(w, r)
	})
}
