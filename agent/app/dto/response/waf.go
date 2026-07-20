package response

// WafSiteStatus separates operator intent from verified data-plane state.
// Protected is true only when the gateway is ready and nginx is actually routed
// through it; Enabled alone never claims protection.
type WafSiteStatus struct {
	WebsiteID uint   `json:"websiteID"`
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Mode      string `json:"mode"`
	Installed bool   `json:"installed"`
	Ready     bool   `json:"ready"`
	Routed    bool   `json:"routed"`
	Protected bool   `json:"protected"`
	LastError string `json:"lastError"`
}
