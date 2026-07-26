package response

import "time"

// WafSiteStatus separates operator intent from verified data-plane state.
// Protected is true only when the gateway is ready and nginx is actually routed
// through it; Enabled alone never claims protection.
type WafSiteStatus struct {
	WebsiteID uint `json:"websiteID"`
	Supported bool `json:"supported"`
	Enabled   bool `json:"enabled"`
	// Mode is the operator's stored intent ("detection", "block", or "inherit");
	// EffectiveMode is the mode the gateway actually applies after resolving
	// "inherit" against the panel-wide default.
	Mode          string   `json:"mode"`
	EffectiveMode string   `json:"effectiveMode"`
	AllowList     []string `json:"allowList"`
	DenyList      []string `json:"denyList"`
	// RateLimits are this site's own overrides; EffectiveRateLimits are what the
	// gateway actually enforces after the panel-wide defaults are merged in. The
	// UI must show the effective set when reporting what is in force.
	RateLimits          []WafRateLimit `json:"rateLimits"`
	EffectiveRateLimits []WafRateLimit `json:"effectiveRateLimits"`
	// Rules is this site's own detection policy (null when it follows the panel
	// default); EffectiveRules is what the gateway actually enforces.
	Rules          *WafRulePolicy `json:"rules"`
	EffectiveRules WafRulePolicy  `json:"effectiveRules"`
	// Region is this site's own geographic access control (null when it follows
	// the panel default); EffectiveRegion is what the gateway actually enforces.
	Region          *WafRegionPolicy `json:"region"`
	EffectiveRegion WafRegionPolicy  `json:"effectiveRegion"`
	// GeoAvailable reports whether the IP address database region control needs
	// is actually installed. It is surfaced so the UI can say the control is
	// unavailable instead of offering a switch that cannot take effect.
	GeoAvailable bool   `json:"geoAvailable"`
	Installed    bool   `json:"installed"`
	Ready        bool   `json:"ready"`
	Routed       bool   `json:"routed"`
	Protected    bool   `json:"protected"`
	LastError    string `json:"lastError"`
}

// WafRateLimit mirrors one stored frequency limit.
type WafRateLimit struct {
	Kind      string `json:"kind"`
	PeriodSec int    `json:"periodSec"`
	Threshold int    `json:"threshold"`
	BanSec    int    `json:"banSec"`
	PerURL    bool   `json:"perUrl"`
}

// WafBan is one temporary block currently in force in the gateway.
type WafBan struct {
	IP        string    `json:"ip"`
	Kind      string    `json:"kind"`
	Host      string    `json:"host"`
	WebsiteID uint      `json:"websiteId"`
	BannedAt  time.Time `json:"bannedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// WafBanState is the gateway's live enforcement state.
type WafBanState struct {
	Bans []WafBan `json:"bans"`
	// TrackedCounters and CounterOverflow expose how much rate-limit state the
	// gateway is holding. CounterOverflow means the tracker had to drop windows
	// to stay bounded, so a flood outran it — surfaced rather than hidden,
	// because it looks exactly like an absence of attacks otherwise.
	TrackedCounters int  `json:"trackedCounters"`
	CounterOverflow bool `json:"counterOverflow"`
}

// WafListEntry is one stored black/white list row.
type WafListEntry struct {
	ID      uint   `json:"id"`
	List    string `json:"list"`
	Target  string `json:"target"`
	Match   string `json:"match"`
	Pattern string `json:"pattern"`
	Remark  string `json:"remark"`
	Enabled bool   `json:"enabled"`
}

// WafCustomRule is one stored operator-authored rule. The slice order is the
// evaluation order, so the UI must not re-sort it.
type WafCustomRule struct {
	ID         uint               `json:"id"`
	Name       string             `json:"name"`
	Action     string             `json:"action"`
	Conditions []WafRuleCondition `json:"conditions"`
	Remark     string             `json:"remark"`
	Enabled    bool               `json:"enabled"`
	// Invalid carries a parse error for a row the data plane cannot be given.
	// It is surfaced rather than hidden: a rule shown as fine while it is not
	// being enforced is exactly the failure mode worth avoiding.
	Invalid string `json:"invalid,omitempty"`
}

// WafRuleCondition is one ANDed test inside a custom rule.
type WafRuleCondition struct {
	Field   string `json:"field"`
	Name    string `json:"name,omitempty"`
	Match   string `json:"match,omitempty"`
	Pattern string `json:"pattern"`
	Negate  bool   `json:"negate,omitempty"`
}

// WafIPGroup is a named address set referenced by ipgroup entries.
type WafIPGroup struct {
	ID      uint     `json:"id"`
	Name    string   `json:"name"`
	Entries []string `json:"entries"`
	Remark  string   `json:"remark"`
}

// WafLists is the whole panel-wide list set. Entries and groups are returned
// together because an entry is only meaningful alongside the groups it can
// reference.
type WafLists struct {
	Entries  []WafListEntry `json:"entries"`
	IPGroups []WafIPGroup   `json:"ipGroups"`
}

// WafRulePolicy mirrors a stored detection policy.
type WafRulePolicy struct {
	DisableSQLi      bool     `json:"disableSqli"`
	DisableXSS       bool     `json:"disableXss"`
	Strict           bool     `json:"strict"`
	AllowedMethods   []string `json:"allowedMethods"`
	BannedUploadExts []string `json:"bannedUploadExts"`
}

// WafRegionPolicy mirrors a stored geographic access policy.
type WafRegionPolicy struct {
	Mode    string   `json:"mode"`
	Regions []string `json:"regions"`
}

// WafGlobalConfig echoes the stored panel-wide defaults with canonicalized lists.
type WafGlobalConfig struct {
	DefaultMode  string          `json:"defaultMode"`
	AllowList    []string        `json:"allowList"`
	DenyList     []string        `json:"denyList"`
	RateLimits   []WafRateLimit  `json:"rateLimits"`
	Rules        WafRulePolicy   `json:"rules"`
	Region       WafRegionPolicy `json:"region"`
	GeoAvailable bool            `json:"geoAvailable"`
}
