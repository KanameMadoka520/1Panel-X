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
	"github.com/corazawaf/coraza/v3/types"
)

type Mode string

const (
	ModeBlock     Mode = "block"
	ModeDetection Mode = "detection"

	defaultBodyLimit = 13 << 20
	inMemoryBodyCap  = 128 << 10
)

type Engine struct {
	waf          atomic.Value
	mode         Mode
	bodyLimit    int
	auditLogPath string
	// observer is called for every rule match Coraza logs, in BOTH detection and
	// block mode. It is the only way to see a match in detection mode: nothing is
	// interrupted there, so the response status carries no signal at all.
	observer func(types.MatchedRule)
}

func NewEngine(mode Mode, bodyLimit int) (*Engine, error) {
	return NewEngineWithAudit(mode, bodyLimit, "")
}

func NewEngineWithAudit(mode Mode, bodyLimit int, auditLogPath string) (*Engine, error) {
	return NewEngineWithObserver(mode, bodyLimit, auditLogPath, nil)
}

func NewEngineWithObserver(mode Mode, bodyLimit int, auditLogPath string, observer func(types.MatchedRule)) (*Engine, error) {
	e := &Engine{mode: mode, bodyLimit: bodyLimit, auditLogPath: auditLogPath, observer: observer}
	if err := e.reloadRaw(directivesFor(mode, bodyLimit, auditLogPath)); err != nil {
		return nil, fmt.Errorf("waf init failed: %w", err)
	}
	return e, nil
}

func (e *Engine) WAF() coraza.WAF {
	return e.waf.Load().(coraza.WAF)
}

// enginePolicy is the set of per-site settings that require an INDEPENDENTLY
// compiled Coraza engine. Sites that resolve to the same policy share one
// compiled instance: a full CRS v4 compile loads a thousand-plus rules and their
// lookup data files, so instances are a real memory and startup cost and their
// number is capped rather than left to grow with the site count.
//
// A zero BodyLimit means "inherit the process default", so a site that only
// overrides its mode still collapses onto the shared base engine.
type enginePolicy struct {
	Mode      Mode
	BodyLimit int
}

func (p enginePolicy) String() string {
	if p.BodyLimit <= 0 {
		return string(p.Mode)
	}
	return fmt.Sprintf("%s/body=%d", p.Mode, p.BodyLimit)
}

// ForMode returns an independently compiled engine with the same body, audit and
// observer settings. It is used when site policies mix detection and block mode.
func (e *Engine) ForMode(mode Mode) (*Engine, error) {
	return NewEngineWithObserver(mode, e.bodyLimit, e.auditLogPath, e.observer)
}

// ForPolicy compiles a sibling engine for one resolved per-site policy. The
// observer is inherited, so a site on a non-default policy is still observed.
func (e *Engine) ForPolicy(p enginePolicy) (*Engine, error) {
	bodyLimit := p.BodyLimit
	if bodyLimit <= 0 {
		bodyLimit = e.bodyLimit
	}
	return NewEngineWithObserver(p.Mode, bodyLimit, e.auditLogPath, e.observer)
}

// maxCompiledEngines bounds how many distinct per-site policies one gateway may
// compile. Exceeding it is reported at config-generation time — naming the site
// that overflowed — instead of quietly compiling one full ruleset per site.
const maxCompiledEngines = 8

// engineCache hands out one compiled engine per distinct policy, seeded with the
// process-wide base engine so the common case (every site on the global default)
// compiles exactly once.
type engineCache struct {
	base     *Engine
	compiled map[enginePolicy]*Engine
	order    []enginePolicy
}

func newEngineCache(base *Engine, basePolicy enginePolicy) *engineCache {
	return &engineCache{
		base:     base,
		compiled: map[enginePolicy]*Engine{basePolicy: base},
		order:    []enginePolicy{basePolicy},
	}
}

func (c *engineCache) get(p enginePolicy, host string) (*Engine, error) {
	if e, ok := c.compiled[p]; ok {
		return e, nil
	}
	if len(c.compiled) >= maxCompiledEngines {
		return nil, fmt.Errorf(
			"site %q needs distinct WAF policy %s but at most %d policies can be compiled (already compiled: %v); merge site policies onto a shared default",
			host, p, maxCompiledEngines, c.order)
	}
	e, err := c.base.ForPolicy(p)
	if err != nil {
		return nil, fmt.Errorf("compile site %q policy %s: %w", host, p, err)
	}
	c.compiled[p] = e
	c.order = append(c.order, p)
	return e, nil
}

func (c *engineCache) size() int { return len(c.compiled) }

func (e *Engine) Reload(mode Mode, bodyLimit int) error {
	if err := e.reloadRaw(directivesFor(mode, bodyLimit, e.auditLogPath)); err != nil {
		return fmt.Errorf("waf reload rejected (keeping running engine): %w", err)
	}
	e.mode = mode
	e.bodyLimit = bodyLimit
	return nil
}

func (e *Engine) reloadRaw(directives string) error {
	cfg := coraza.NewWAFConfig().WithRootFS(coreruleset.FS).WithDirectives(directives)
	if e.observer != nil {
		cfg = cfg.WithErrorCallback(e.observer)
	}
	waf, err := coraza.NewWAF(cfg)
	if err != nil {
		return err
	}
	e.waf.Store(waf)
	return nil
}

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
	return base + fmt.Sprintf(`SecAuditEngine RelevantOnly
SecAuditLogParts ABHZ
SecAuditLogFormat json
SecAuditLogType Serial
SecAuditLog %s
`, auditLogPath)
}
