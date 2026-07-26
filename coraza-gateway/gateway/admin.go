package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// AdminTokenHeader carries the shared secret on management requests.
const AdminTokenHeader = "X-Waf-Admin-Token"

// adminPathPrefix is served on the same loopback listener as /healthz. Because
// WithAdmin wraps the router, these paths are resolved BEFORE any site is
// dispatched, so no protected website can ever shadow them.
const adminPathPrefix = "/admin/"

// AdminState is the gateway's live enforcement state.
type AdminState struct {
	Bans []BanEntry `json:"bans"`
	// TrackedCounters is how many rate-limit windows are currently held, and
	// CounterOverflow reports that the tracker had to drop windows to stay within
	// its memory bound. Reporting it keeps a flood that outruns the tracker
	// visible instead of looking like an absence of attacks.
	TrackedCounters int  `json:"trackedCounters"`
	CounterOverflow bool `json:"counterOverflow"`
}

type releaseRequest struct {
	IP string `json:"ip"`
}

type releaseResponse struct {
	Released bool `json:"released"`
}

// WithAdmin gates the management API behind two independent checks.
//
// The loopback Host check alone is NOT sufficient: the gateway runs with
// network_mode host, so any process on the box — including another container —
// can satisfy it. Releasing a ban is a security-relevant mutation, so it also
// requires a shared token that the control plane writes to a file only it and
// the gateway can read.
//
// An empty token disables the management API entirely rather than leaving it
// open: no capability is safer than an ungated one.
func WithAdmin(handler http.Handler, enforcer *Enforcer, token string) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" || enforcer == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, adminPathPrefix) && isLoopbackHealthHost(normalizeHost(r.Host)) {
				http.Error(w, "management API disabled", http.StatusNotFound)
				return
			}
			handler.ServeHTTP(w, r)
		})
	}
	expected := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, adminPathPrefix) {
			handler.ServeHTTP(w, r)
			return
		}
		if !isLoopbackHealthHost(normalizeHost(r.Host)) {
			// Not an admin request at all as far as a protected site is concerned:
			// fall through so a website that happens to use /admin/ still works.
			handler.ServeHTTP(w, r)
			return
		}
		presented := []byte(r.Header.Get(AdminTokenHeader))
		if subtle.ConstantTimeCompare(presented, expected) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		serveAdmin(w, r, enforcer)
	})
}

func serveAdmin(w http.ResponseWriter, r *http.Request, enforcer *Enforcer) {
	switch r.URL.Path {
	case "/admin/state":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeAdminJSON(w, AdminState{
			Bans:            enforcer.Bans(),
			TrackedCounters: enforcer.limiter.size(),
			CounterOverflow: enforcer.limiter.overflowed(),
		})
	case "/admin/bans/release":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req releaseRequest
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		ip := strings.TrimSpace(req.IP)
		if ip == "" {
			http.Error(w, "ip is required", http.StatusBadRequest)
			return
		}
		_, released := enforcer.Release(ip)
		writeAdminJSON(w, releaseResponse{Released: released})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func writeAdminJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

// ReadAdminToken loads the shared secret. A missing or unreadable file disables
// the management API rather than failing startup: the WAF's job is inspecting
// traffic, and it must keep doing that even when management is unavailable.
func ReadAdminToken(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("coraza-gateway: management API disabled (%v)", err)
		return ""
	}
	return strings.TrimSpace(string(data))
}
