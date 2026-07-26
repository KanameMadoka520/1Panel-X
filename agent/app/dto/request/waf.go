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
	// Rules, when present, becomes this site's OWN detection policy and replaces
	// the panel-wide default wholesale. Absent means the site keeps following the
	// panel default.
	Rules *WafRulePolicy `json:"rules"`
	// Region follows the same whole-or-nothing rule as Rules.
	Region *WafRegionPolicy `json:"region"`
}

// WafRegionPolicy is geographic access control. Regions are ISO 3166-1 alpha-2
// country codes; an empty list means no region control at all, whatever Mode
// says, so an unfinished form cannot lock every visitor out.
type WafRegionPolicy struct {
	Mode    string   `json:"mode" validate:"omitempty,oneof=allow deny"`
	Regions []string `json:"regions" validate:"omitempty,max=512,dive,len=2"`
}

// WafRulePolicy is a detection policy. Fields are "disable" flags so the zero
// value is the fully-protecting one.
type WafRulePolicy struct {
	DisableSQLi bool `json:"disableSqli"`
	DisableXSS  bool `json:"disableXss"`
	Strict      bool `json:"strict"`
	// AllowedMethods is the HTTP method allow-list; empty leaves the rule set's
	// own default in force. Tokens are validated server-side because they end up
	// inside an engine directive.
	AllowedMethods []string `json:"allowedMethods" validate:"omitempty,max=64,dive,max=20"`
	// BannedUploadExts are extensions refused when they name an uploaded file.
	// Validated server-side for the same reason as the methods above: they end up
	// inside an engine directive.
	BannedUploadExts []string `json:"bannedUploadExts" validate:"omitempty,max=64,dive,max=16"`
}

// WafListEntrySave creates or updates one panel-wide black/white list row.
// ID 0 creates. Match is ignored for address targets.
type WafListEntrySave struct {
	ID      uint   `json:"id"`
	List    string `json:"list" validate:"required,oneof=deny allow"`
	Target  string `json:"target" validate:"required,oneof=ip ipgroup url ua"`
	Match   string `json:"match" validate:"omitempty,oneof=exact prefix suffix contains regex"`
	Pattern string `json:"pattern" validate:"required,max=512"`
	Remark  string `json:"remark" validate:"omitempty,max=256"`
	Enabled bool   `json:"enabled"`
}

// WafCustomRuleSave creates or updates one operator-authored rule. ID 0 creates.
type WafCustomRuleSave struct {
	ID         uint               `json:"id"`
	Name       string             `json:"name" validate:"omitempty,max=64"`
	Action     string             `json:"action" validate:"required,oneof=deny allow log"`
	Conditions []WafRuleCondition `json:"conditions" validate:"required,min=1,max=8,dive"`
	Remark     string             `json:"remark" validate:"omitempty,max=256"`
	Enabled    bool               `json:"enabled"`
}

// WafRuleCondition is one ANDed test inside a custom rule.
type WafRuleCondition struct {
	Field   string `json:"field" validate:"required,oneof=ip host method uri path query ua referer header cookie"`
	Name    string `json:"name" validate:"omitempty,max=64"`
	Match   string `json:"match" validate:"omitempty,oneof=exact prefix suffix contains regex"`
	Pattern string `json:"pattern" validate:"required,max=512"`
	Negate  bool   `json:"negate"`
}

// WafCustomRuleReorder rewrites the evaluation order. The whole id sequence is
// sent rather than a move instruction, so the stored order can never drift from
// what the operator is looking at.
type WafCustomRuleReorder struct {
	IDs []uint `json:"ids" validate:"required,min=1,max=200"`
}

// WafIPGroupSave creates or updates a named address set.
type WafIPGroupSave struct {
	ID      uint     `json:"id"`
	Name    string   `json:"name" validate:"required,max=64"`
	Entries []string `json:"entries" validate:"omitempty,max=4096,dive,max=64"`
	Remark  string   `json:"remark" validate:"omitempty,max=256"`
}

// WafBanRelease lifts a temporary ban ahead of its expiry.
type WafBanRelease struct {
	IP string `json:"ip" validate:"required,ip"`
}

// WafListDelete removes list rows or IP groups by id.
type WafListDelete struct {
	IDs []uint `json:"ids" validate:"required,min=1,max=500"`
}

// WafGlobalUpdate replaces the panel-wide WAF defaults: the mode applied to
// "inherit" sites plus IP lists merged into every enabled site.
type WafGlobalUpdate struct {
	DefaultMode string           `json:"defaultMode" validate:"required,oneof=detection block"`
	AllowList   []string         `json:"allowList" validate:"omitempty,max=512,dive,max=64"`
	DenyList    []string         `json:"denyList" validate:"omitempty,max=512,dive,max=64"`
	RateLimits  []WafRateLimit   `json:"rateLimits" validate:"omitempty,max=8,dive"`
	Rules       *WafRulePolicy   `json:"rules"`
	Region      *WafRegionPolicy `json:"region"`
}
