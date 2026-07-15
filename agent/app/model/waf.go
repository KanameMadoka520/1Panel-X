package model

import "time"

// WafAttackEvent is one WAF attack event ingested from the coraza-gateway audit
// log (one blocked/flagged request). Every attacker-controlled field has already
// been sanitized (control-char strip + length cap) by agent/utils/wafaudit
// before it is stored (W6).
type WafAttackEvent struct {
	BaseModel
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
