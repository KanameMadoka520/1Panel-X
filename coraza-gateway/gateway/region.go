package gateway

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// Region access control resolves the client address to a country and applies an
// operator allow- or deny-list.
//
// The honesty constraint that shapes this file: a region policy is only
// meaningful if the address database is actually there. A missing database must
// therefore FAIL the config rather than degrade to "everything passes" — a panel
// reporting "region restriction: on" while every request sails through is the
// exact failure this whole project refuses to ship.

// RegionMode says how the region list is read.
type RegionMode string

const (
	// RegionAllow admits ONLY the listed regions.
	RegionAllow RegionMode = "allow"
	// RegionDeny admits everything except the listed regions.
	RegionDeny RegionMode = "deny"
)

const maxRegionEntries = 512

// regionCodePattern is an ISO 3166-1 alpha-2 country code. Granularity stops at
// the country: the bundled database exposes a province name but no stable
// province code, and matching on a display name would silently break the moment
// the database's wording changed.
var regionCodePattern = regexp.MustCompile(`^[A-Za-z]{2}$`)

// RegionPolicy is one site's region access control. An empty Regions list means
// no region control at all, whatever Mode says, so an unfinished form cannot
// lock every visitor out.
type RegionPolicy struct {
	Mode    RegionMode `json:"mode,omitempty"`
	Regions []string   `json:"regions,omitempty"`
}

// IsZero reports the "no region control" policy.
func (p *RegionPolicy) IsZero() bool { return p == nil || len(p.Regions) == 0 }

// normalizeRegions upper-cases, de-duplicates and sorts the country codes.
func normalizeRegions(regions []string) ([]string, error) {
	if len(regions) == 0 {
		return nil, nil
	}
	if len(regions) > maxRegionEntries {
		return nil, fmt.Errorf("too many regions (%d), limit is %d", len(regions), maxRegionEntries)
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

func (p *RegionPolicy) validate() error {
	if p == nil {
		return nil
	}
	regions, err := normalizeRegions(p.Regions)
	if err != nil {
		return err
	}
	p.Regions = regions
	if len(regions) == 0 {
		p.Mode = ""
		return nil
	}
	switch p.Mode {
	case RegionAllow, RegionDeny:
	case "":
		p.Mode = RegionDeny
	default:
		return fmt.Errorf("invalid region mode %q", p.Mode)
	}
	return nil
}

// geoRecord is the subset of the bundled database this gateway reads. It matches
// the schema the panel already uses for its own IP lookups.
type geoRecord struct {
	ISO string `maxminddb:"iso"`
}

// GeoDB resolves addresses to country codes. A nil *GeoDB is a database that is
// not available, and every caller must treat that as "cannot enforce", never as
// "nothing to enforce".
type GeoDB struct {
	reader *maxminddb.Reader
	mu     sync.RWMutex
	cache  map[string]string
}

// maxGeoCacheEntries bounds the lookup cache. Past the cap the cache is dropped
// wholesale rather than evicted one by one: it is a latency optimisation, and a
// simple reset cannot develop a subtle eviction bug that pins the wrong entry.
const maxGeoCacheEntries = 8192

// OpenGeoDB opens the address database.
//
// An empty path, or a path where no file exists, means "no database available"
// and yields a nil *GeoDB with no error — most installations have no region
// policy at all, and refusing to start over a file they never needed would be
// absurd. Nothing is lost by being lenient here: a site that DOES configure a
// region policy is refused by newRegionMatcher, naming the missing database.
//
// A file that exists but cannot be read or parsed IS an error, because that is
// an operator who installed a database that does not work.
func OpenGeoDB(path string) (*GeoDB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		log.Printf("coraza-gateway: no IP address database at %s; region access control is unavailable", path)
		return nil, nil
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open IP address database %q: %w", path, err)
	}
	log.Printf("coraza-gateway: IP address database loaded from %s", path)
	return &GeoDB{reader: reader, cache: make(map[string]string)}, nil
}

func (g *GeoDB) Close() error {
	if g == nil || g.reader == nil {
		return nil
	}
	return g.reader.Close()
}

// Country returns the ISO country code for an address, or "" when the database
// cannot place it.
func (g *GeoDB) Country(ip net.IP) string {
	if g == nil || g.reader == nil || ip == nil {
		return ""
	}
	key := ip.String()
	g.mu.RLock()
	cached, ok := g.cache[key]
	g.mu.RUnlock()
	if ok {
		return cached
	}
	var record geoRecord
	if err := g.reader.Lookup(ip, &record); err != nil {
		record.ISO = ""
	}
	iso := strings.ToUpper(strings.TrimSpace(record.ISO))
	g.mu.Lock()
	if len(g.cache) >= maxGeoCacheEntries {
		g.cache = make(map[string]string)
	}
	g.cache[key] = iso
	g.mu.Unlock()
	return iso
}

// countryResolver places an address. It exists so the decision logic can be
// tested without shipping a binary address database into the repository.
type countryResolver interface {
	Country(net.IP) string
}

// regionMatcher evaluates one site's region policy.
type regionMatcher struct {
	mode    RegionMode
	regions map[string]struct{}
	geo     countryResolver
}

func (m *regionMatcher) empty() bool { return m == nil || len(m.regions) == 0 }

// newRegionMatcher compiles a policy against the address database.
//
// A policy with no database is a HARD ERROR, not a warning: the alternative is a
// gateway that reports itself ready while enforcing nothing of what the operator
// configured.
func newRegionMatcher(p *RegionPolicy, geo *GeoDB, host string) (*regionMatcher, error) {
	if p.IsZero() {
		return nil, nil
	}
	// Checked on the concrete type: a nil *GeoDB stored in the interface below
	// would not compare equal to nil, and the policy would look enforceable.
	if geo == nil || geo.reader == nil {
		return nil, fmt.Errorf(
			"site %q has a region access policy but no IP address database is available; install the address database or remove the region policy",
			host)
	}
	return newRegionMatcherWith(p, geo)
}

func newRegionMatcherWith(p *RegionPolicy, geo countryResolver) (*regionMatcher, error) {
	if p.IsZero() {
		return nil, nil
	}
	regions := make(map[string]struct{}, len(p.Regions))
	for _, r := range p.Regions {
		regions[strings.ToUpper(strings.TrimSpace(r))] = struct{}{}
	}
	mode := p.Mode
	if mode == "" {
		mode = RegionDeny
	}
	return &regionMatcher{mode: mode, regions: regions, geo: geo}, nil
}

// refuses reports whether this address is outside the permitted regions, along
// with the country that produced the decision.
func (m *regionMatcher) refuses(ip net.IP) (bool, string) {
	if m.empty() || ip == nil {
		return false, ""
	}
	// A private, loopback or link-local address is not on the public internet, so
	// a geographic policy has nothing to say about it. Refusing these would break
	// container health checks and internal callers for no security gain.
	if !isPublicIP(ip) {
		return false, ""
	}
	country := m.geo.Country(ip)
	_, listed := m.regions[country]
	if m.mode == RegionAllow {
		// An address the database cannot place is outside every permitted region.
		// "Only these countries" has to mean exactly that, or the control is
		// trivially defeated by any address the database happens not to know.
		return !listed, country
	}
	return listed, country
}

// isPublicIP reports whether an address is globally routable.
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	// Carrier-grade NAT (100.64.0.0/10) is not routable on the public internet
	// either, and a client behind it carries no meaningful location.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}
