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
	WebsiteID uint   `gorm:"uniqueIndex" json:"websiteID"`
	Enabled   bool   `json:"enabled"`
	Mode      string `gorm:"size:16;not null;default:detection" json:"mode"`
	// AllowIPs and DenyIPs are newline-separated canonical IP/CIDR entries. Allow
	// entries are trusted clients that bypass CRS inspection but are still
	// proxied; deny entries are refused with a 403 regardless of Mode (deny wins).
	AllowIPs  string `gorm:"type:text" json:"allowIPs"`
	DenyIPs   string `gorm:"type:text" json:"denyIPs"`
	LastError string `gorm:"size:2048" json:"lastError"`
}
