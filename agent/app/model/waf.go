package model

import "time"

// WafAttackEvent is one WAF attack event ingested from the coraza-gateway audit
// log (one blocked/flagged request). Every attacker-controlled field has already
// been sanitized (control-char strip + length cap) by agent/utils/wafaudit
// before it is stored (W6).
type WafAttackEvent struct {
	BaseModel
	TxID        string    `gorm:"uniqueIndex" json:"txID"`
	WebsiteID   uint      `gorm:"index:idx_waf_site_time" json:"websiteID"`
	Time        time.Time `gorm:"index:idx_waf_site_time" json:"time"`
	Host        string    `json:"host"`
	SourceIP    string    `gorm:"index" json:"sourceIP"`
	Method      string    `json:"method"`
	URI         string    `json:"uri"`
	RuleID      int       `json:"ruleID"`
	RuleMsg     string    `json:"ruleMsg"`
	Category    string    `gorm:"index" json:"category"`
	Severity    string    `json:"severity"`
	MatchedData string    `json:"matchedData"`
	HitCount    int       `json:"hitCount"`
	Action      string    `json:"action"` // "blocked" | "detected"
}

// WafAuditCursor tracks how far the audit log has been ingested (one row per
// audit-file path) so the tailer only reads new bytes.
type WafAuditCursor struct {
	BaseModel
	Path   string `gorm:"uniqueIndex" json:"path"`
	Offset int64  `json:"offset"`
}

// WafSitePolicy is the control-plane intent for one managed website. The actual
// protected state is derived from gateway readiness plus nginx routing, never
// inferred from Enabled alone.
type WafSitePolicy struct {
	BaseModel
	WebsiteID uint `gorm:"uniqueIndex" json:"websiteID"`
	Enabled   bool `json:"enabled"`
	// Mode is "detection", "block", or "inherit" (follow WafGlobalPolicy).
	// "inherit" is stored as an explicit sentinel rather than an empty string:
	// gorm omits zero-valued fields carrying a default tag on insert, so "" would
	// silently become the column default and lose the operator's intent.
	Mode string `gorm:"size:16;not null;default:detection" json:"mode"`
	// AllowIPs and DenyIPs are newline-separated canonical IP/CIDR entries. Allow
	// entries are trusted clients that bypass CRS inspection but are still
	// proxied; deny entries are refused with a 403 regardless of Mode (deny wins).
	AllowIPs string `gorm:"type:text" json:"allowIPs"`
	DenyIPs  string `gorm:"type:text" json:"denyIPs"`
	// RateLimits is a JSON array of this site's frequency limits. A site entry
	// replaces the panel-wide entry of the same kind; kinds it does not mention
	// keep the panel-wide value.
	RateLimits string `gorm:"type:text" json:"rateLimits"`
	// RuleOptions is this site's detection policy as JSON. Empty means the site
	// has stored none of its own and inherits the panel-wide default whole.
	RuleOptions string `gorm:"type:text" json:"ruleOptions"`
	LastError   string `gorm:"size:2048" json:"lastError"`
}

// WafListEntry is one row of the panel-wide black/white lists.
//
// The lists are panel-wide rather than per-site, matching how they are presented
// as a single top-level set. A site's own IP list remains available as an
// additional, narrower control layered underneath these.
type WafListEntry struct {
	BaseModel
	// List is "deny" (blacklist) or "allow" (whitelist).
	List string `gorm:"size:8;not null;index" json:"list"`
	// Target is "ip", "ipgroup", "url" or "ua".
	Target string `gorm:"size:16;not null;index" json:"target"`
	// Match is "exact", "prefix", "contains" or "regex"; empty for address targets.
	Match   string `gorm:"size:16" json:"match"`
	Pattern string `gorm:"size:512;not null" json:"pattern"`
	Remark  string `gorm:"size:256" json:"remark"`
	// Enabled rows are the only ones sent to the gateway, so switching a row off
	// stops it being enforced instead of merely hiding it.
	Enabled bool `gorm:"not null;default:true" json:"enabled"`
}

// WafCustomRule is one operator-authored condition/action rule.
//
// Rules are panel-wide and ORDER MATTERS: the data plane resolves the first
// matching rule, so Priority is the operator's own ordering and is part of the
// policy rather than a display preference.
type WafCustomRule struct {
	BaseModel
	Name string `gorm:"size:64" json:"name"`
	// Action is "deny", "allow" or "log".
	Action string `gorm:"size:8;not null;default:deny" json:"action"`
	// Conditions is the JSON array of ANDed conditions. It is stored as JSON
	// rather than as rows because a rule is only ever read, written and validated
	// whole; splitting it would let a partially-written rule reach the data plane.
	Conditions string `gorm:"type:text;not null" json:"conditions"`
	Priority   int    `gorm:"not null;default:0;index" json:"priority"`
	Remark     string `gorm:"size:256" json:"remark"`
	// Enabled rows are the only ones sent to the gateway, so switching a rule off
	// stops it being enforced instead of merely hiding it.
	Enabled bool `gorm:"not null;default:true" json:"enabled"`
}

// WafIPGroup is a named address set that list entries can reference, so one
// shared set can back several rules.
type WafIPGroup struct {
	BaseModel
	Name string `gorm:"size:64;uniqueIndex;not null" json:"name"`
	// Entries is newline-separated canonical IP/CIDR text.
	Entries string `gorm:"type:text" json:"entries"`
	Remark  string `gorm:"size:256" json:"remark"`
}

// WafGlobalPolicy is the panel-wide default WAF policy, kept as a single row.
// DefaultMode applies to sites whose policy mode is "inherit"; AllowIPs/DenyIPs
// are merged into every enabled site's lists at config generation (deny still
// wins over allow at the gateway).
type WafGlobalPolicy struct {
	BaseModel
	DefaultMode string `gorm:"size:16;not null;default:detection" json:"defaultMode"`
	AllowIPs    string `gorm:"type:text" json:"allowIPs"`
	DenyIPs     string `gorm:"type:text" json:"denyIPs"`
	// RateLimits is a JSON array of the panel-wide frequency limits. The attack
	// limit can ONLY live here: rule matches are observed through the shared
	// engine's callback, which reports no Host, so a per-site attack threshold
	// would be a number the data plane cannot honour.
	RateLimits string `gorm:"type:text" json:"rateLimits"`
	// RuleOptions is the panel-wide detection policy as JSON.
	RuleOptions string `gorm:"type:text" json:"ruleOptions"`
}
