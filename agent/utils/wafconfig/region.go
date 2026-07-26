package wafconfig

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Region access control vocabulary. These mirror the data plane's contract; the
// gateway validates them again on load, so a mismatch fails closed.
const (
	// RegionAllow admits ONLY the listed regions.
	RegionAllow = "allow"
	// RegionDeny admits everything except the listed regions.
	RegionDeny = "deny"

	MaxRegionEntries = 512
)

// regionCodePattern is an ISO 3166-1 alpha-2 country code.
//
// Granularity deliberately stops at the country. The bundled address database
// carries a province NAME but no stable province code, and matching on a display
// name would break silently the first time that wording changed — a region rule
// that quietly stops matching is worse than one that was never offered.
var regionCodePattern = regexp.MustCompile(`^[A-Za-z]{2}$`)

// RegionPolicy is one site's geographic access control.
type RegionPolicy struct {
	Mode string `json:"mode,omitempty"`
	// Regions are ISO 3166-1 alpha-2 country codes. Empty means no region
	// control at all, whatever Mode says.
	Regions []string `json:"regions,omitempty"`
	// Disabled switches the control off while KEEPING the region list, which is
	// what the panel's master toggle does. It is a negative so its zero value
	// means "active": a stored policy that predates this field was written by an
	// operator who meant it to apply.
	Disabled bool `json:"disabled,omitempty"`
}

// IsZero reports a policy the data plane has nothing to do with, which is
// emitted as an absent object so the config stays small.
func (p *RegionPolicy) IsZero() bool {
	return p == nil || len(p.Regions) == 0 || p.Disabled
}

// NormalizeRegionPolicy validates and canonicalizes a policy. Canonical ordering
// keeps the config-generation digest stable for an unchanged policy.
func NormalizeRegionPolicy(p RegionPolicy) (RegionPolicy, error) {
	regions, err := NormalizeRegions(p.Regions)
	if err != nil {
		return RegionPolicy{}, err
	}
	p.Regions = regions
	if len(regions) == 0 {
		p.Mode = ""
		return p, nil
	}
	switch strings.TrimSpace(p.Mode) {
	case RegionAllow:
		p.Mode = RegionAllow
	case RegionDeny:
		p.Mode = RegionDeny
	case "":
		// A list of countries with no mode is far more likely to mean "keep these
		// out" than "keep everyone else out", so the conservative reading wins.
		p.Mode = RegionDeny
	default:
		return RegionPolicy{}, fmt.Errorf("invalid region mode %q", p.Mode)
	}
	return p, nil
}

// NormalizeRegions upper-cases, de-duplicates and sorts the country codes.
func NormalizeRegions(regions []string) ([]string, error) {
	if len(regions) == 0 {
		return nil, nil
	}
	if len(regions) > MaxRegionEntries {
		return nil, fmt.Errorf("too many regions (%d), limit is %d", len(regions), MaxRegionEntries)
	}
	seen := make(map[string]struct{}, len(regions))
	out := make([]string, 0, len(regions))
	for _, r := range regions {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if !regionCodePattern.MatchString(r) {
			return nil, fmt.Errorf("invalid region %q (expected a two-letter country code such as CN or US)", r)
		}
		r = strings.ToUpper(r)
		if _, dup := seen[r]; dup {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

// MergeRegionPolicy overlays a site's policy on the panel-wide default.
//
// As with the detection policy, the site's policy is taken WHOLE rather than
// merged field by field: an empty region list cannot be told apart from "not
// set", so per-field merging would make it impossible for a site to switch
// region control back off once the panel default switched it on.
func MergeRegionPolicy(global, site *RegionPolicy) *RegionPolicy {
	if site != nil {
		return site
	}
	return global
}
