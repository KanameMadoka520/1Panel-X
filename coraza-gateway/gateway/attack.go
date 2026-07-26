package gateway

import (
	"sync"
	"sync/atomic"

	"github.com/corazawaf/coraza/v3/types"
)

// maxAttackDedupeEntries bounds the set of transactions already counted.
// Transactions are short-lived, so a few thousand entries covers every request
// in flight with room to spare.
const maxAttackDedupeEntries = 8192

// attackDeduper collapses the many rule matches Coraza reports for one request
// into a single counted attack.
//
// CRS is an anomaly-scoring ruleset: one hostile request routinely matches
// several rules (the detection rule, the score evaluator, and so on). Counting
// each callback would inflate an attack-frequency threshold by an unpredictable
// factor, so matches are deduplicated by transaction id.
type attackDeduper struct {
	mu   sync.Mutex
	seen map[string]struct{}
	fifo []string
	max  int
}

func newAttackDeduper(max int) *attackDeduper {
	if max <= 0 {
		max = maxAttackDedupeEntries
	}
	return &attackDeduper{seen: make(map[string]struct{}, max), fifo: make([]string, 0, max), max: max}
}

// first reports whether this is the first match seen for the transaction.
func (d *attackDeduper) first(txID string) bool {
	if txID == "" {
		// Without an id there is nothing to correlate on; counting it is the
		// conservative choice — under-counting attacks would weaken the limit.
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[txID]; ok {
		return false
	}
	if len(d.fifo) >= d.max {
		oldest := d.fifo[0]
		d.fifo = d.fifo[1:]
		delete(d.seen, oldest)
	}
	d.seen[txID] = struct{}{}
	d.fifo = append(d.fifo, txID)
	return true
}

// attackLimit is the gateway-wide attack-frequency limit.
//
// It is deliberately NOT per-site. The only place a rule match is observable in
// detection mode is Coraza's error callback, which reports the client address
// and the rule but not the Host, and the compiled engine is shared by every site
// on the same policy — so a per-site threshold here would be a number the data
// plane cannot actually honour. Keeping it gateway-wide matches how the bans it
// produces already work, and the UI says so rather than implying otherwise.
type attackLimit struct {
	cfg atomic.Pointer[RateLimitConfig]
}

func (l *attackLimit) set(cfg *RateLimitConfig) { l.cfg.Store(cfg) }

func (l *attackLimit) get() *RateLimitConfig { return l.cfg.Load() }

// AttackObserver returns the callback to hand to the Coraza engine.
func (e *Enforcer) AttackObserver() func(types.MatchedRule) {
	if e == nil {
		return nil
	}
	return func(rule types.MatchedRule) {
		e.recordAttack(rule.TransactionID(), rule.ClientIPAddress())
	}
}

// SetAttackLimit installs the gateway-wide attack limit from the current config.
// A nil config disables it without discarding counters already accumulated.
func (e *Enforcer) SetAttackLimit(cfg *RateLimitConfig) {
	if e == nil {
		return
	}
	e.attack.set(cfg)
}

func (e *Enforcer) recordAttack(txID, ip string) {
	if e == nil || ip == "" {
		return
	}
	cfg := e.attack.get()
	if cfg == nil {
		return
	}
	if !e.deduper.first(txID) {
		return
	}
	// The attack limit is gateway-wide, so it is counted against an empty site
	// scope; the resulting ban is global exactly like every other ban.
	out := e.count(siteRef{}, *cfg, ip, "")
	if out.Triggered && !out.Banned {
		e.journal.Record(EnforcementEvent{
			Kind:     EventRateLimit,
			ClientIP: ip,
			Rule:     out.Rule,
			Action:   "detected",
		})
	}
}
