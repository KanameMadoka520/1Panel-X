package gateway

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// defaultReloadInterval is how often the gateway re-reads its config file. The
// file is small (one entry per protected host) and is only re-parsed when its
// content digest changes, so a tick costs one read and one hash.
//
// Polling is used instead of inotify because the config arrives through a
// read-only bind mount into a distroless container, where filesystem events are
// not reliably delivered, and because the agent replaces the file by rename —
// a watcher registered on the old inode would stop seeing updates entirely.
const defaultReloadInterval = 2 * time.Second

// RouterBuilder compiles a validated config into a live Router. It is injected
// so the reload machinery can be tested without a real CRS compile per case.
type RouterBuilder func(Config) (*Router, error)

// ReloadStatus is the observable outcome of the most recent config load.
type ReloadStatus struct {
	Generation string
	Sites      int
	// LastError is why the most recent CANDIDATE config was rejected. It is
	// non-empty only while a rejected config sits on disk. Traffic is still
	// protected by the last accepted config, so this means "the running policy is
	// older than the file on disk" — never "the gateway stopped inspecting".
	// The control plane distinguishes "not applied yet" from "refused" by reading
	// this together with Generation.
	LastError string
}

// ReloadableRouter serves traffic from the most recently ACCEPTED config while a
// background watcher re-reads the config file.
//
// A candidate that fails to parse or compile is discarded and the running router
// is kept — the same compile-then-swap discipline the engine already uses (W9),
// so a truncated or invalid write can never drop protection.
//
// Reloading in-process (rather than restarting the container on every save) is
// what makes stateful enforcement possible at all: rate-limit counters and
// temporary IP bans live in this process's memory, and restarting on every
// unrelated policy save would silently erase them.
type ReloadableRouter struct {
	router atomic.Pointer[Router]
	status atomic.Pointer[ReloadStatus]

	path     string
	build    RouterBuilder
	mode     Mode
	interval time.Duration
	logf     func(string, ...any)

	// mu guards the attempt bookkeeping so a manual reload and the watcher can
	// never interleave two candidate swaps.
	mu         sync.Mutex
	attempted  [sha256.Size]byte
	hasAttempt bool
}

// NewReloadableRouter loads path once and fails if that first config is not
// usable: startup must be fail-closed, because there is no previous good config
// to fall back to.
func NewReloadableRouter(path string, mode Mode, build RouterBuilder) (*ReloadableRouter, error) {
	rr := &ReloadableRouter{
		path:     path,
		build:    build,
		mode:     mode,
		interval: defaultReloadInterval,
		logf:     log.Printf,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := rr.applyLocked(data); err != nil {
		return nil, err
	}
	return rr, nil
}

func (rr *ReloadableRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rr.router.Load().ServeHTTP(w, r)
}

// HealthSnapshot reports the RUNNING configuration, not the file on disk.
func (rr *ReloadableRouter) HealthSnapshot() Health {
	st := rr.status.Load()
	return Health{
		Status:     "ready",
		Sites:      st.Sites,
		Mode:       rr.mode,
		Generation: st.Generation,
		LastError:  st.LastError,
	}
}

// ReloadFromFile re-reads the config and swaps in a new router if the file
// changed and the candidate compiles. Unchanged content is a no-op, and an
// already-attempted digest is not retried, so a persistently bad file neither
// spins nor floods the log.
func (rr *ReloadableRouter) ReloadFromFile() error {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	data, err := os.ReadFile(rr.path)
	if err != nil {
		// A transient read failure must not discard the running config, and it is
		// deliberately not recorded as a rejection: the file may reappear on the
		// next tick (the agent replaces it by rename).
		return err
	}
	digest := sha256.Sum256(data)
	if rr.hasAttempt && digest == rr.attempted {
		return nil
	}
	rr.attempted = digest
	rr.hasAttempt = true

	if err := rr.applyLocked(data); err != nil {
		rr.logf("coraza-gateway: rejected new config, keeping running policy: %v", err)
		return err
	}
	st := rr.status.Load()
	rr.logf("coraza-gateway: applied config generation=%s sites=%d", st.Generation, st.Sites)
	return nil
}

// applyLocked parses and compiles a candidate, swapping it in only on success.
// The caller holds rr.mu (or is the constructor, which is not yet shared).
func (rr *ReloadableRouter) applyLocked(data []byte) error {
	cfg, err := ParseConfig(data)
	if err != nil {
		rr.recordRejection(err)
		return err
	}
	rt, err := rr.build(cfg)
	if err != nil {
		rr.recordRejection(err)
		return err
	}
	rr.router.Store(rt)
	rr.status.Store(&ReloadStatus{Generation: cfg.Generation, Sites: len(cfg.Sites)})
	rr.attempted = sha256.Sum256(data)
	rr.hasAttempt = true
	return nil
}

// recordRejection keeps the previously accepted generation/site count visible —
// those describe what is actually running — and only adds the rejection reason.
func (rr *ReloadableRouter) recordRejection(err error) {
	next := ReloadStatus{LastError: err.Error()}
	if prev := rr.status.Load(); prev != nil {
		next.Generation = prev.Generation
		next.Sites = prev.Sites
	}
	rr.status.Store(&next)
}

// Watch re-reads the config until ctx is cancelled. Reload failures are recorded
// on /healthz rather than returned, because there is no caller to return them to
// and the running policy is still being enforced.
func (rr *ReloadableRouter) Watch(ctx context.Context) {
	ticker := time.NewTicker(rr.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = rr.ReloadFromFile()
		}
	}
}

// Describe is used for the startup log line.
func (rr *ReloadableRouter) Describe() string {
	st := rr.status.Load()
	return fmt.Sprintf("%d sites (watching %s)", st.Sites, rr.path)
}
