package gateway

import (
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
)

// fakeGeo places addresses without needing a binary database in the repository.
// Anything it does not know is reported as unplaceable, which is the case the
// allow-mode semantics turn on.
type fakeGeo map[string]string

func (f fakeGeo) Country(ip net.IP) string { return f[ip.String()] }

func buildRegionHandler(t *testing.T, p *RegionPolicy, geo countryResolver, reached *bool, journal *EventJournal) *Handler {
	t.Helper()
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newRegionMatcherWith(p, geo)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(engine, recordingUpstream(reached), ModeBlock).
		WithRegion(m).
		WithJournal(journal)
}

func serveFromAddr(h http.Handler, remoteAddr string) int {
	return customRequest(h, "GET", "/", nil, remoteAddr).Code
}

// The load-bearing assertion for deny mode: the listed country is refused and
// never reaches the origin, everyone else passes.
func TestRegionDenyModeRefusesListedCountries(t *testing.T) {
	var reached bool
	geo := fakeGeo{"203.0.113.7": "RU", "198.51.100.7": "US"}
	h := buildRegionHandler(t, &RegionPolicy{Mode: RegionDeny, Regions: []string{"ru"}}, geo, &reached, nil)

	reached = false
	if code := serveFromAddr(h, "203.0.113.7:1234"); code != http.StatusForbidden {
		t.Fatalf("a listed country must be refused, got %d", code)
	}
	if reached {
		t.Fatal("a refused client must not reach the origin")
	}
	reached = false
	if code := serveFromAddr(h, "198.51.100.7:1234"); code != http.StatusOK || !reached {
		t.Fatalf("an unlisted country must pass, got %d reached=%v", code, reached)
	}
}

// Allow mode admits ONLY the listed countries — including refusing an address
// the database cannot place. Anything else would let the control be defeated by
// any client the database happens not to know.
func TestRegionAllowModeAdmitsOnlyListedCountries(t *testing.T) {
	var reached bool
	geo := fakeGeo{"203.0.113.7": "CN", "198.51.100.7": "US"}
	h := buildRegionHandler(t, &RegionPolicy{Mode: RegionAllow, Regions: []string{"CN"}}, geo, &reached, nil)

	reached = false
	if code := serveFromAddr(h, "203.0.113.7:1234"); code != http.StatusOK || !reached {
		t.Fatalf("a permitted country must pass, got %d reached=%v", code, reached)
	}
	reached = false
	if code := serveFromAddr(h, "198.51.100.7:1234"); code != http.StatusForbidden {
		t.Fatalf("a country outside the allow list must be refused, got %d", code)
	}
	// 192.0.2.9 is public but absent from the fake database.
	reached = false
	if code := serveFromAddr(h, "192.0.2.9:1234"); code != http.StatusForbidden {
		t.Fatalf("an unplaceable address must be refused under allow mode, got %d", code)
	}
}

// A private or loopback client is not on the public internet, so a geographic
// policy has nothing to say about it. Refusing these would break container
// health checks and internal callers for no security gain.
func TestRegionControlIgnoresNonPublicAddresses(t *testing.T) {
	var reached bool
	h := buildRegionHandler(t, &RegionPolicy{Mode: RegionAllow, Regions: []string{"CN"}}, fakeGeo{}, &reached, nil)

	for _, addr := range []string{"127.0.0.1:1234", "10.1.2.3:1234", "192.168.1.9:1234", "172.16.0.5:1234", "100.64.0.1:1234"} {
		reached = false
		if code := serveFromAddr(h, addr); code != http.StatusOK || !reached {
			t.Fatalf("%s is not on the public internet and must pass, got %d reached=%v", addr, code, reached)
		}
	}
}

// This is the honesty guard: a region policy with no address database must FAIL
// the config, not come up enforcing nothing while the panel reports it active.
func TestRegionPolicyWithoutDatabaseIsRefused(t *testing.T) {
	_, err := newRegionMatcher(&RegionPolicy{Mode: RegionAllow, Regions: []string{"CN"}}, nil, "a.example")
	if err == nil {
		t.Fatal("a region policy without an address database must fail rather than silently do nothing")
	}
	if !strings.Contains(err.Error(), "a.example") {
		t.Fatalf("the error must name the offending site, got %v", err)
	}
	// The same must hold through the router, which is where it actually matters.
	engine, engineErr := NewEngine(ModeBlock, 1<<20)
	if engineErr != nil {
		t.Fatal(engineErr)
	}
	cfg := Config{Sites: []SiteConfig{{
		Host: "a.example", Upstream: "http://127.0.0.1:1",
		Region: &RegionPolicy{Mode: RegionAllow, Regions: []string{"CN"}},
	}}}
	if _, err := NewRouter(cfg, engine, ModeBlock, ""); err == nil {
		t.Fatal("the router must refuse a region policy it cannot enforce")
	}
	// A site with NO region policy must still build without a database, or every
	// installation would need one just to run the WAF.
	plain := Config{Sites: []SiteConfig{{Host: "a.example", Upstream: "http://127.0.0.1:1"}}}
	if _, err := NewRouter(plain, engine, ModeBlock, ""); err != nil {
		t.Fatalf("a site without a region policy must not need an address database: %v", err)
	}
}

// Most installations have no region policy and no address database. Refusing to
// start over a file they never needed would be absurd — and nothing is lost,
// because a site that DOES configure a policy is refused by the router above.
func TestMissingAddressDatabaseIsNotFatalOnItsOwn(t *testing.T) {
	geo, err := OpenGeoDB(t.TempDir() + "/absent.mmdb")
	if err != nil {
		t.Fatalf("a missing database must not be an error on its own: %v", err)
	}
	if geo != nil {
		t.Fatal("a missing database must yield no reader")
	}
	// A file that exists but is not a valid database IS an error: that is an
	// operator who installed something broken, not one who installed nothing.
	broken := t.TempDir() + "/broken.mmdb"
	if err := os.WriteFile(broken, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenGeoDB(broken); err == nil {
		t.Fatal("an unreadable database must be reported, not ignored")
	}
}

// The master toggle switches the control off while KEEPING the list, and it is
// stored as a NEGATIVE so a policy written before the field existed stays in
// force. A new field must never silently switch an operator's policy off.
func TestRegionMasterToggle(t *testing.T) {
	var reached bool
	geo := fakeGeo{"203.0.113.7": "RU"}
	off := &RegionPolicy{Mode: RegionDeny, Regions: []string{"RU"}, Disabled: true}

	if !off.IsZero() {
		t.Fatal("a switched-off policy must restrict nothing")
	}
	h := buildRegionHandler(t, off, geo, &reached, nil)
	reached = false
	if code := serveFromAddr(h, "203.0.113.7:1234"); code != http.StatusOK || !reached {
		t.Fatalf("a switched-off policy must let the request through, got %d reached=%v", code, reached)
	}
	// Absent field (the pre-existing config shape) keeps the policy active.
	cfg, err := ParseConfig([]byte(`{"sites":[{"host":"a.example","upstream":"http://127.0.0.1:1",
		"region":{"mode":"deny","regions":["RU"]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sites[0].Region.IsZero() {
		t.Fatal("a policy written before the toggle existed must stay in force")
	}
	// A switched-off policy is still validated, so turning it back on cannot
	// surprise the operator with a value that was never usable.
	bad := &RegionPolicy{Mode: "sideways", Regions: []string{"RU"}, Disabled: true}
	if err := bad.validate(); err == nil {
		t.Fatal("a switched-off policy must still be validated")
	}
	// And it needs no address database while it is off.
	if _, err := newRegionMatcher(off, nil, "a.example"); err != nil {
		t.Fatalf("a switched-off policy must not require the address database: %v", err)
	}
}

// An empty region list means no control at all, whatever the mode says, so an
// unfinished form cannot lock every visitor out.
func TestEmptyRegionListIsNoControl(t *testing.T) {
	m, err := newRegionMatcherWith(&RegionPolicy{Mode: RegionAllow}, fakeGeo{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.empty() {
		t.Fatal("an empty region list must not restrict anything")
	}
	// And it must not require a database either.
	if _, err := newRegionMatcher(&RegionPolicy{Mode: RegionAllow}, nil, "a.example"); err != nil {
		t.Fatalf("an empty region policy must not need an address database: %v", err)
	}
}

func TestRegionPolicyValidation(t *testing.T) {
	p := &RegionPolicy{Mode: RegionAllow, Regions: []string{" cn ", "US", "cn", ""}}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.Regions, ",") != "CN,US" {
		t.Fatalf("regions must be upper-cased, de-duplicated and sorted, got %v", p.Regions)
	}
	for _, bad := range []string{"CHN", "C", "中国", "cn,us", "*"} {
		p := &RegionPolicy{Mode: RegionDeny, Regions: []string{bad}}
		if err := p.validate(); err == nil {
			t.Fatalf("region %q must be rejected", bad)
		}
	}
	if err := (&RegionPolicy{Mode: "sometimes", Regions: []string{"CN"}}).validate(); err == nil {
		t.Fatal("an unknown region mode must be rejected")
	}
	// An omitted mode defaults to deny, which is the conservative reading: a list
	// of countries with no mode is far more likely to mean "keep these out" than
	// "keep everyone else out".
	q := &RegionPolicy{Regions: []string{"CN"}}
	if err := q.validate(); err != nil || q.Mode != RegionDeny {
		t.Fatalf("an omitted mode must default to deny, got %q (%v)", q.Mode, err)
	}
	if err := (&RegionPolicy{Regions: make([]string, maxRegionEntries+1)}).validate(); err == nil {
		t.Fatal("an oversized region list must be rejected")
	}
}

// An explicit operator allow outranks the geographic policy, the same way it
// outranks bans and frequency limits: it is a deliberate exemption.
func TestExplicitAllowOutranksRegionControl(t *testing.T) {
	var reached bool
	geo := fakeGeo{"198.51.100.7": "US"}
	m, err := newRegionMatcherWith(&RegionPolicy{Mode: RegionAllow, Regions: []string{"CN"}}, geo)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(ModeBlock, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	acl, err := newIPACL([]string{"198.51.100.7"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(engine, recordingUpstream(&reached), ModeBlock).WithRegion(m).WithIPACL(acl)

	reached = false
	if code := serveFromAddr(h, "198.51.100.7:1234"); code != http.StatusOK || !reached {
		t.Fatalf("an allow-listed client must not be region-restricted, got %d reached=%v", code, reached)
	}
}

func TestRegionRefusalIsRecordedWithTheCountry(t *testing.T) {
	var reached bool
	dir := t.TempDir()
	journal := NewEventJournal(dir + "/events.log")
	defer journal.Close()

	geo := fakeGeo{"203.0.113.7": "RU"}
	h := buildRegionHandler(t, &RegionPolicy{Mode: RegionDeny, Regions: []string{"RU"}}, geo, &reached, journal)
	if code := serveFromAddr(h, "203.0.113.7:1234"); code != http.StatusForbidden {
		t.Fatalf("expected a refusal, got %d", code)
	}

	var found bool
	for _, e := range readJournal(t, dir+"/events.log") {
		if e.Kind == EventRegion {
			found = true
			if e.Rule != "region:RU" || e.Action != "blocked" {
				t.Fatalf("the record must name the country that produced the decision: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("a region refusal must leave a record")
	}
}
