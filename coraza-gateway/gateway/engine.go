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
}

func NewEngine(mode Mode, bodyLimit int) (*Engine, error) {
	return NewEngineWithAudit(mode, bodyLimit, "")
}

func NewEngineWithAudit(mode Mode, bodyLimit int, auditLogPath string) (*Engine, error) {
	e := &Engine{mode: mode, bodyLimit: bodyLimit, auditLogPath: auditLogPath}
	if err := e.reloadRaw(directivesFor(mode, bodyLimit, auditLogPath)); err != nil {
		return nil, fmt.Errorf("waf init failed: %w", err)
	}
	return e, nil
}

func (e *Engine) WAF() coraza.WAF {
	return e.waf.Load().(coraza.WAF)
}

// ForMode returns an independently compiled engine with the same body and audit
// settings. It is used when site policies mix detection and block mode.
func (e *Engine) ForMode(mode Mode) (*Engine, error) {
	return NewEngineWithAudit(mode, e.bodyLimit, e.auditLogPath)
}

func (e *Engine) Reload(mode Mode, bodyLimit int) error {
	if err := e.reloadRaw(directivesFor(mode, bodyLimit, e.auditLogPath)); err != nil {
		return fmt.Errorf("waf reload rejected (keeping running engine): %w", err)
	}
	e.mode = mode
	e.bodyLimit = bodyLimit
	return nil
}

func (e *Engine) reloadRaw(directives string) error {
	waf, err := coraza.NewWAF(coraza.NewWAFConfig().WithRootFS(coreruleset.FS).WithDirectives(directives))
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
