package response

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
	Installed           bool           `json:"installed"`
	Ready               bool           `json:"ready"`
	Routed              bool           `json:"routed"`
	Protected           bool           `json:"protected"`
	LastError           string         `json:"lastError"`
}

// WafRateLimit mirrors one stored frequency limit.
type WafRateLimit struct {
	Kind      string `json:"kind"`
	PeriodSec int    `json:"periodSec"`
	Threshold int    `json:"threshold"`
	BanSec    int    `json:"banSec"`
	PerURL    bool   `json:"perUrl"`
}

// WafGlobalConfig echoes the stored panel-wide defaults with canonicalized lists.
type WafGlobalConfig struct {
	DefaultMode string         `json:"defaultMode"`
	AllowList   []string       `json:"allowList"`
	DenyList    []string       `json:"denyList"`
	RateLimits  []WafRateLimit `json:"rateLimits"`
}
