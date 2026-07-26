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

// WafRateLimit is one frequency limit. BanSec 0 means the limit is recorded but
// does not ban — the per-limit equivalent of the engine's detection mode.
type WafRateLimit struct {
	Kind      string `json:"kind" validate:"required,oneof=access url notfound attack"`
	PeriodSec int    `json:"periodSec" validate:"required,min=1,max=3600"`
	Threshold int    `json:"threshold" validate:"required,min=1"`
	BanSec    int    `json:"banSec" validate:"min=0,max=86400"`
	PerURL    bool   `json:"perUrl"`
}

type WafSiteUpdate struct {
	Enabled bool `json:"enabled"`
	// "inherit" makes the site follow the panel-wide default mode.
	Mode string `json:"mode" validate:"required,oneof=detection block inherit"`
	// AllowList/DenyList are IP or CIDR entries. They are normalized and validated
	// server-side; the per-entry/length caps here only bound obviously abusive
	// input before that authoritative check.
	AllowList []string `json:"allowList" validate:"omitempty,max=512,dive,max=64"`
	DenyList  []string `json:"denyList" validate:"omitempty,max=512,dive,max=64"`
	// RateLimits override the panel-wide limits of the same kind. The "attack"
	// kind is refused here: it is enforced gateway-wide and can only be set on
	// the global policy.
	RateLimits []WafRateLimit `json:"rateLimits" validate:"omitempty,max=8,dive"`
}

// WafGlobalUpdate replaces the panel-wide WAF defaults: the mode applied to
// "inherit" sites plus IP lists merged into every enabled site.
type WafGlobalUpdate struct {
	DefaultMode string         `json:"defaultMode" validate:"required,oneof=detection block"`
	AllowList   []string       `json:"allowList" validate:"omitempty,max=512,dive,max=64"`
	DenyList    []string       `json:"denyList" validate:"omitempty,max=512,dive,max=64"`
	RateLimits  []WafRateLimit `json:"rateLimits" validate:"omitempty,max=8,dive"`
}
