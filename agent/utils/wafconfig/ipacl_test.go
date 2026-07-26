package wafconfig

import (
	"reflect"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
)

func TestNormalizeIPListCanonicalizesDedupesSorts(t *testing.T) {
	got, err := NormalizeIPList([]string{" 10.0.0.5/8 ", "1.2.3.4", "1.2.3.4", "", "2001:DB8::1", "192.168.1.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	// Canonicalized (masked CIDR, lowercased IPv6), de-duplicated, sorted lexically.
	wantSorted := []string{"1.2.3.4", "10.0.0.0/8", "192.168.1.0/24", "2001:db8::1"}
	if !reflect.DeepEqual(got, wantSorted) {
		t.Fatalf("got %#v want %#v", got, wantSorted)
	}
}

func TestNormalizeIPListRejectsInvalid(t *testing.T) {
	for _, bad := range []string{"not-an-ip", "1.2.3.4/40", "10.0.0.0/8/8", "300.1.1.1"} {
		if _, err := NormalizeIPList([]string{bad}); err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
}

func TestNormalizeIPListEmptyIsEmpty(t *testing.T) {
	got, err := NormalizeIPList([]string{"", "   "})
	if err != nil || len(got) != 0 {
		t.Fatalf("blank-only list should normalize to empty: got=%#v err=%v", got, err)
	}
}

func TestBuildAttachesAndCanonicalizesIPLists(t *testing.T) {
	input := []Site{{
		Website:  model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
		Domains:  []model.WebsiteDomain{{Domain: "a.example"}, {Domain: "b.example"}},
		Enabled:  true,
		AllowIPs: []string{"10.0.0.9"},
		DenyIPs:  []string{"203.0.113.0/24", "203.0.113.0/24"},
	}}
	cfg, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sites) != 2 {
		t.Fatalf("expected 2 host entries, got %d", len(cfg.Sites))
	}
	for _, s := range cfg.Sites {
		if !reflect.DeepEqual(s.AllowIPs, []string{"10.0.0.9"}) {
			t.Fatalf("allow list not attached to every host entry: %#v", s)
		}
		if !reflect.DeepEqual(s.DenyIPs, []string{"203.0.113.0/24"}) {
			t.Fatalf("deny list must be canonicalized+deduped per host: %#v", s.DenyIPs)
		}
	}
}

func TestBuildGenerationChangesWithIPLists(t *testing.T) {
	mk := func(deny ...string) GatewayConfig {
		cfg, err := Build([]Site{{
			Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
			Domains: []model.WebsiteDomain{{Domain: "a.example"}},
			Enabled: true,
			DenyIPs: deny,
		}})
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	base := mk()
	withDeny := mk("1.2.3.4")
	if base.Generation == withDeny.Generation {
		t.Fatal("adding a deny entry must change the config generation hash")
	}
	// Same effective rule set (different input order/case) → same generation.
	a := mk("1.2.3.4", "10.0.0.0/8")
	b := mk("10.0.0.5/8", "1.2.3.4")
	if a.Generation != b.Generation {
		t.Fatalf("equivalent lists must produce a stable generation: %s vs %s", a.Generation, b.Generation)
	}
}

func TestBuildRejectsInvalidIPList(t *testing.T) {
	_, err := Build([]Site{{
		Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
		Domains: []model.WebsiteDomain{{Domain: "a.example"}},
		Enabled: true,
		DenyIPs: []string{"bogus"},
	}})
	if err == nil {
		t.Fatal("Build must reject an invalid deny entry")
	}
}
