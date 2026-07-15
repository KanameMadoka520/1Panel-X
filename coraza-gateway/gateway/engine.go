// Package gateway is the CI-verifiable heart of the 1Panel-X community WAF: a
// real OWASP Coraza v3 + CRS v4 engine wrapped around an HTTP reverse proxy.
// It is a genuine engine — NOT a shim — so it can be unit-tested by firing
// attack payloads at the handler and asserting they are blocked, with no live
// nginx or site. See .planning/research/WAF-ENGINE-DESIGN.md (controls W1-W12).
package gateway

import (
	"fmt"
	"sync/atomic"

	coreruleset "github.com/corazawaf/coraza-coreruleset/v4"
	"github.com/corazawaf/coraza/v3"
)

// Mode selects enforcement behaviour.
//
//	ModeBlock     — disruptive rules return 403 (fail-closed on engine error, W1).
//	ModeDetection — rules only log; requests pass (explicitly-labelled fail-open).
type Mode string

const (
	ModeBlock     Mode = "block"
	ModeDetection Mode = "detection"

	defaultBodyLimit = 13 << 20  // 13 MiB (W3: bound request-body buffering)
	inMemoryBodyCap  = 128 << 10 // 128 KiB kept in memory before spooling
)

// Engine holds the live Coraza WAF behind an atomic pointer so a ruleset reload
// can swap it without locking the request path.
type Engine struct {
	waf          atomic.Value // stores coraza.WAF
	mode         Mode
	bodyLimit    int
	auditLogPath string // "" disables the attack-event audit log
}

// NewEngine compiles the CRS-backed ruleset for the given mode. A compile
// failure is returned (never a half-built engine).
func NewEngine(mode Mode, bodyLimit int) (*Engine, error) {
	return NewEngineWithAudit(mode, bodyLimit, "")
}

// NewEngineWithAudit is NewEngine plus a JSON audit log: when auditLogPath is
// non-empty, Coraza appends one JSON record per rule-matching transaction (in
// both block and detection mode) that the agent tails into the attack-event
// store (Phase 20). An empty path disables audit logging.
func NewEngineWithAudit(mode Mode, bodyLimit int, auditLogPath string) (*Engine, error) {
	e := &Engine{mode: mode, bodyLimit: bodyLimit, auditLogPath: auditLogPath}
	if err := e.reloadRaw(directivesFor(mode, bodyLimit, auditLogPath)); err != nil {
		return nil, fmt.Errorf("waf init failed: %w", err)
	}
	return e, nil
}

// WAF returns the currently-live engine (safe for concurrent request handling).
func (e *Engine) WAF() coraza.WAF {
	return e.waf.Load().(coraza.WAF)
}

// Reload compiles a candidate engine and only swaps it in if it builds cleanly
// (W9: compile-then-swap). On failure the running engine is kept and the error
// returned, so a bad ruleset can never take the WAF down or leave it half-loaded.
func (e *Engine) Reload(mode Mode, bodyLimit int) error {
	if err := e.reloadRaw(directivesFor(mode, bodyLimit, e.auditLogPath)); err != nil {
		return fmt.Errorf("waf reload rejected (keeping running engine): %w", err)
	}
	e.mode = mode
	e.bodyLimit = bodyLimit
	return nil
}

// reloadRaw builds a WAF from arbitrary directives and atomically swaps it in on
// success only. Kept unexported so tests can exercise the compile-then-swap
// guarantee with a deliberately-broken ruleset.
func (e *Engine) reloadRaw(directives string) error {
	waf, err := coraza.NewWAF(
		coraza.NewWAFConfig().
			WithRootFS(coreruleset.FS).
			WithDirectives(directives),
	)
	if err != nil {
		return err
	}
	e.waf.Store(waf)
	return nil
}

// directivesFor renders the SecLang config. The CRS includes come first; our
// engine/body directives come LAST so they win over the recommended defaults.
func directivesFor(mode Mode, bodyLimit int, auditLogPath string) string {
	engineDirective := "SecRuleEngine On"
	if mode == ModeDetection {
		engineDirective = "SecRuleEngine DetectionOnly"
	}
	if bodyLimit <= 0 {
		bodyLimit = defaultBodyLimit
	}
	inMem := bodyLimit
	if inMem > inMemoryBodyCap {
		inMem = inMemoryBodyCap
	}
	base := fmt.Sprintf(`Include @coraza.conf-recommended
Include @crs-setup.conf.example
Include @owasp_crs/*.conf
%s
SecRequestBodyAccess On
SecRequestBodyLimit %d
SecRequestBodyInMemoryLimit %d
SecRequestBodyLimitAction Reject
`, engineDirective, bodyLimit, inMem)

	if auditLogPath == "" {
		return base
	}
	// RelevantOnly → only rule-matching transactions are logged; JSON serial →
	// one self-contained JSON object per line for the agent tailer. Parts A
	// (transaction/request meta) + B (request headers, incl. Host) + H (matched
	// rule messages) + Z (terminator) are what the attack-event parser needs.
	return base + fmt.Sprintf(`SecAuditEngine RelevantOnly
SecAuditLogParts ABHZ
SecAuditLogFormat json
SecAuditLogType Serial
SecAuditLog %s
`, auditLogPath)
}
