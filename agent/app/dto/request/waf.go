package request

import "time"

// WafEventSearch queries a website's WAF attack events. An empty range defaults
// to the last 7 days; Category optionally filters (e.g. "sqli", "xss").
type WafEventSearch struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Category  string    `json:"category" validate:"omitempty,max=32"`
	Page      int       `json:"page"`
	PageSize  int       `json:"pageSize"`
}

type WafSiteUpdate struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode" validate:"required,oneof=detection block"`
	// AllowList/DenyList are IP or CIDR entries. They are normalized and validated
	// server-side; the per-entry/length caps here only bound obviously abusive
	// input before that authoritative check.
	AllowList []string `json:"allowList" validate:"omitempty,max=512,dive,max=64"`
	DenyList  []string `json:"denyList" validate:"omitempty,max=512,dive,max=64"`
}
