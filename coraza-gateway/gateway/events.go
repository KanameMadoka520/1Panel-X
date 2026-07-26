package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EventKind classifies a NON-CRS enforcement decision.
//
// CRS matches are recorded by Coraza's own audit log; everything the gateway
// decides on its own used to leave no trace at all, which made those decisions
// invisible to the panel — an IP refused by the operator's deny list produced a
// 403 and nothing else. These records close that gap, and they are deliberately
// kept in a separate journal so the Coraza audit schema stays untouched.
type EventKind string

const (
	EventACLDeny     EventKind = "acl-deny"
	EventUnknownHost EventKind = "unknown-host"
	EventOversize    EventKind = "oversize-body"
	EventRateLimit   EventKind = "ratelimit"
	EventBan         EventKind = "ban"
	EventBanned      EventKind = "banned"
	EventBanReleased EventKind = "ban-released"
)

const (
	// maxEventFieldBytes bounds attacker-controlled strings so a flood of long
	// URIs cannot blow up the journal. The control plane sanitizes again on
	// ingest; this is the cheap first cut at the write side.
	maxEventFieldBytes = 2048
	// maxEventJournalBytes caps the journal so a sustained attack cannot fill the
	// audit volume. Past the cap, writing stops rather than rotating: dropping
	// NEW records is recoverable, whereas discarding already-written ones the
	// control plane has not ingested yet would lose them permanently.
	maxEventJournalBytes = 256 << 20
)

// EnforcementEvent is one non-CRS decision, serialized as a single JSON line.
type EnforcementEvent struct {
	// ID is stable per written line, so the control plane can re-read a region of
	// the journal after a crash without double-counting.
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	Kind      EventKind `json:"kind"`
	WebsiteID uint      `json:"websiteId,omitempty"`
	Host      string    `json:"host,omitempty"`
	ClientIP  string    `json:"clientIp,omitempty"`
	Method    string    `json:"method,omitempty"`
	URI       string    `json:"uri,omitempty"`
	// Rule names the specific list, limit, or rule that fired.
	Rule string `json:"rule,omitempty"`
	// Action is what actually happened to the request: "blocked" when it was
	// refused, "detected" when it was recorded but allowed through, "banned" and
	// "released" for ban lifecycle records. It must reflect the real outcome,
	// never the configured intent.
	Action string `json:"action"`
	// Detail carries record-specific structured context (e.g. a ban's expiry).
	Detail string `json:"detail,omitempty"`
}

// EventJournal appends enforcement events as JSON lines. A journal that cannot
// be opened degrades to a no-op: losing visibility is bad, but refusing traffic
// because a log file is unavailable would be worse.
type EventJournal struct {
	mu      sync.Mutex
	file    *os.File
	written int64
	nonce   string
	seq     uint64
	now     func() time.Time
	full    bool
}

// NewEventJournal opens (or creates) the journal at path. An empty path, or a
// path that cannot be opened, yields a nil journal, which is safe to use.
func NewEventJournal(path string) *EventJournal {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		log.Printf("coraza-gateway: enforcement journal disabled (%v)", err)
		return nil
	}
	size := int64(0)
	if st, statErr := f.Stat(); statErr == nil {
		size = st.Size()
	}
	return &EventJournal{file: f, written: size, nonce: newJournalNonce(), now: time.Now}
}

// newJournalNonce makes IDs unique across process restarts. It is only an
// identity prefix, never a secret, so a failed read degrades to a fixed value
// rather than taking the gateway down.
func newJournalNonce() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(buf[:])
}

// Record writes one event. Fields are truncated first; JSON encoding already
// escapes newlines, so a crafted URI cannot forge an extra journal line.
func (j *EventJournal) Record(e EnforcementEvent) {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.full {
		return
	}

	j.seq++
	e.ID = j.nonce + "-" + strconv.FormatUint(j.seq, 10)
	if e.Time.IsZero() {
		e.Time = j.now().UTC()
	}
	e.Host = truncateField(e.Host)
	e.ClientIP = truncateField(e.ClientIP)
	e.Method = truncateField(e.Method)
	e.URI = truncateField(e.URI)
	e.Rule = truncateField(e.Rule)

	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')
	if j.written+int64(len(line)) > maxEventJournalBytes {
		j.full = true
		log.Printf("coraza-gateway: enforcement journal reached %d bytes; not recording further events", maxEventJournalBytes)
		return
	}
	n, err := j.file.Write(line)
	j.written += int64(n)
	if err != nil {
		log.Printf("coraza-gateway: enforcement journal write failed: %v", err)
	}
}

func (j *EventJournal) Close() error {
	if j == nil || j.file == nil {
		return nil
	}
	return j.file.Close()
}

func truncateField(s string) string {
	if len(s) <= maxEventFieldBytes {
		return s
	}
	return s[:maxEventFieldBytes]
}

// siteRef identifies which protected site a decision belongs to. Router fills it
// in; the single-upstream mode leaves it zero.
type siteRef struct {
	WebsiteID uint
	Host      string
}

// recordEvent is the handler-side helper that stamps in the request context.
func (h *Handler) recordEvent(r *http.Request, kind EventKind, rule, action string) {
	if h.journal == nil {
		return
	}
	h.journal.Record(EnforcementEvent{
		Kind:      kind,
		WebsiteID: h.site.WebsiteID,
		Host:      h.site.Host,
		ClientIP:  clientIPString(r.RemoteAddr),
		Method:    r.Method,
		URI:       r.URL.RequestURI(),
		Rule:      rule,
		Action:    action,
	})
}

// clientIPString returns the evaluated client IP without its synthetic port.
func clientIPString(remoteAddr string) string {
	if ip := clientIP(remoteAddr); ip != nil {
		return ip.String()
	}
	return ""
}
