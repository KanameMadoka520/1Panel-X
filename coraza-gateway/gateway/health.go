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
	// LastError reports that a candidate config on disk was REJECTED and the
	// gateway is still enforcing the previous one. Generation always describes
	// what is RUNNING, so the control plane can tell "not applied yet" (generation
	// still old, no error) from "refused" (generation still old, error set).
	LastError string `json:"lastError,omitempty"`
}

// HealthSource yields the current health snapshot. A static config reports fixed
// values; a ReloadableRouter reports whatever it is running right now.
type HealthSource interface {
	HealthSnapshot() Health
}

type staticHealth struct{ health Health }

func (s staticHealth) HealthSnapshot() Health { return s.health }

func WithHealth(handler http.Handler, sites int, mode Mode, generation string) http.Handler {
	return WithHealthSource(handler, staticHealth{Health{
		Status:     "ready",
		Sites:      sites,
		Mode:       mode,
		Generation: generation,
	}})
}

func WithHealthSource(handler http.Handler, source HealthSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" && isLoopbackHealthHost(normalizeHost(r.Host)) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(source.HealthSnapshot())
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func isLoopbackHealthHost(host string) bool {
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
