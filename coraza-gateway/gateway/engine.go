// Package gateway is the CI-verifiable heart of the 1Panel-X community WAF: a
// real OWASP Coraza v3 + CRS v4 engine wrapped around an HTTP reverse proxy.
// It is a genuine engine — NOT a shim — so it can be unit-tested by firing
// attack payloads at the handler and asserting they are blocked, with no live
// nginx or site. See .planning/research/WAF-ENGINE-DESIGN.md (controls W1-W12).
package gateway

import (
	"fmt"
	"strings"
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
	// policy is the detection policy this instance was compiled with.
	policy enginePolicy
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
	return newEngine(enginePolicy{Mode: mode, BodyLimit: bodyLimit}, auditLogPath, observer)
}

func newEngine(policy enginePolicy, auditLogPath string, observer func(types.MatchedRule)) (*Engine, error) {
	e := &Engine{
		mode:         policy.Mode,
		bodyLimit:    policy.BodyLimit,
		auditLogPath: auditLogPath,
		observer:     observer,
		policy:       policy,
	}
	if err := e.reloadRaw(directivesFor(policy, auditLogPath)); err != nil {
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
	// DisableSQLi and DisableXSS are stored as NEGATIVES on purpose: the zero
	// value of this struct must be the fully-protecting policy, so a site that
	// configures nothing can never end up with detection silently switched off.
	DisableSQLi bool
	DisableXSS  bool
	// Strict raises the CRS paranoia level, applying stricter checks at the cost
	// of more false positives.
	Strict bool
	// AllowedMethods is the canonical space-separated HTTP method allow-list.
	// Empty leaves the ruleset's own default in force.
	AllowedMethods string
}

func (p enginePolicy) String() string {
	parts := []string{string(p.Mode)}
	if p.BodyLimit > 0 {
		parts = append(parts, fmt.Sprintf("body=%d", p.BodyLimit))
	}
	if p.DisableSQLi {
		parts = append(parts, "sqli=off")
	}
	if p.DisableXSS {
		parts = append(parts, "xss=off")
	}
	if p.Strict {
		parts = append(parts, "strict")
	}
	if p.AllowedMethods != "" {
		parts = append(parts, "methods="+p.AllowedMethods)
	}
	return strings.Join(parts, "/")
}

// ForMode returns an independently compiled engine with the same body, audit and
// observer settings. It is used when site policies mix detection and block mode.
func (e *Engine) ForMode(mode Mode) (*Engine, error) {
	p := e.policy
	p.Mode = mode
	return newEngine(p, e.auditLogPath, e.observer)
}

// ForPolicy compiles a sibling engine for one resolved per-site policy. The
// observer is inherited, so a site on a non-default policy is still observed.
func (e *Engine) ForPolicy(p enginePolicy) (*Engine, error) {
	if p.BodyLimit <= 0 {
		p.BodyLimit = e.bodyLimit
	}
	return newEngine(p, e.auditLogPath, e.observer)
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
	policy := e.policy
	policy.Mode = mode
	policy.BodyLimit = bodyLimit
	if err := e.reloadRaw(directivesFor(policy, e.auditLogPath)); err != nil {
		return fmt.Errorf("waf reload rejected (keeping running engine): %w", err)
	}
	e.mode = mode
	e.bodyLimit = bodyLimit
	e.policy = policy
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

// CRS rule-id ranges the detection toggles map onto. These are the ONLY ranges a
// toggle may remove; the initialization (901), common-exception (905), blocking
// evaluation (949/959) and correlation (980) families are structurally
// load-bearing — removing 949/959 would silently disable ALL blocking while
// every attack-class toggle still read "on".
const (
	crsSQLiRuleRange = "942011-942560"
	crsXSSRuleRange  = "941010-941400"
	// crsSQLLeakageRuleRange is the RESPONSE-side SQL error leakage family. It is
	// deliberately NOT removed together with the request-side SQLi rules: the
	// toggle is named "SQL injection protection", and silently widening it to
	// response inspection would turn one switch into two undisclosed effects.
	crsSQLLeakageRuleRange = "951010-951260"

	// strictParanoiaLevel is what "strict mode" raises CRS to. PL2 adds
	// noticeably stricter checks at the cost of more false positives.
	strictParanoiaLevel = 2

	// setupRuleIDParanoia is our own rule id for the paranoia override. CRS
	// reserves 900000-900999 for setup and ships those rules commented out, but
	// we stay well clear of the whole CRS numbering space so a future ruleset
	// cannot collide with us.
	setupRuleIDParanoia = 8000001
	setupRuleIDMethods  = 8000002
)

func directivesFor(policy enginePolicy, auditLogPath string) string {
	engineDirective := "SecRuleEngine On"
	if policy.Mode == ModeDetection {
		engineDirective = "SecRuleEngine DetectionOnly"
	}
	bodyLimit := policy.BodyLimit
	if bodyLimit <= 0 {
		bodyLimit = defaultBodyLimit
	}
	inMem := bodyLimit
	if inMem > inMemoryBodyCap {
		inMem = inMemoryBodyCap
	}

	var b strings.Builder
	b.WriteString("Include @coraza.conf-recommended\n")
	b.WriteString("Include @crs-setup.conf.example\n")

	// CRS applies its own defaults from REQUEST-901-INITIALIZATION.conf, each
	// guarded by `SecRule &TX:<var> "@eq 0"` — i.e. only when the variable has
	// never been set. Phase-1 rules run in definition order, so an override must
	// be emitted HERE, between the setup include and the rule include. Emitting
	// it afterwards would run too late: 901 would already have installed the
	// default and the consuming rules would already have read it.
	if policy.Strict {
		fmt.Fprintf(&b, "SecAction \"id:%d,phase:1,pass,t:none,nolog,setvar:tx.blocking_paranoia_level=%d\"\n",
			setupRuleIDParanoia, strictParanoiaLevel)
	}
	if policy.AllowedMethods != "" {
		// Enforcement rides CRS rule 911100, which refuses any method outside
		// this list. We never touch 911 itself.
		fmt.Fprintf(&b, "SecAction \"id:%d,phase:1,pass,t:none,nolog,setvar:'tx.allowed_methods=%s'\"\n",
			setupRuleIDMethods, policy.AllowedMethods)
	}

	b.WriteString("Include @owasp_crs/*.conf\n")
	b.WriteString(engineDirective + "\n")
	fmt.Fprintf(&b, "SecRequestBodyAccess On\nSecRequestBodyLimit %d\nSecRequestBodyInMemoryLimit %d\nSecRequestBodyLimitAction Reject\n",
		bodyLimit, inMem)

	if auditLogPath != "" {
		fmt.Fprintf(&b, "SecAuditEngine RelevantOnly\nSecAuditLogParts ABHZ\nSecAuditLogFormat json\nSecAuditLogType Serial\nSecAuditLog %s\n", auditLogPath)
	}

	// Removals must come AFTER the rules are loaded. Ranges are used rather than
	// individual ids because SecRuleRemoveById treats an absent id as a silent
	// no-op, so a CRS upgrade that renumbers or drops rules can never brick the
	// configuration. SecRuleUpdateActionById is deliberately never emitted: it
	// hard-errors on a missing id, which would take every site down on a ruleset
	// bump, and its multi-id form only applies the first id.
	if policy.DisableSQLi {
		fmt.Fprintf(&b, "SecRuleRemoveById %s\n", crsSQLiRuleRange)
	}
	if policy.DisableXSS {
		fmt.Fprintf(&b, "SecRuleRemoveById %s\n", crsXSSRuleRange)
	}
	return b.String()
}
