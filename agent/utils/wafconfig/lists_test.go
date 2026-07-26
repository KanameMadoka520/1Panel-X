package wafconfig

import (
	"reflect"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
)

func TestNormalizeIPGroupsCanonicalizesAndSorts(t *testing.T) {
	got, err := NormalizeIPGroups([]IPGroup{
		{Name: " scanners ", Entries: []string{"10.0.0.5/8", "1.2.3.4", "1.2.3.4"}},
		{Name: "office", Entries: []string{"203.0.113.7"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []IPGroup{
		{Name: "office", Entries: []string{"203.0.113.7"}},
		{Name: "scanners", Entries: []string{"1.2.3.4", "10.0.0.0/8"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestNormalizeIPGroupsRejectsInvalid(t *testing.T) {
	cases := map[string][]IPGroup{
		"empty name":     {{Name: "   ", Entries: []string{"1.2.3.4"}}},
		"duplicate name": {{Name: "g", Entries: []string{"1.2.3.4"}}, {Name: "g"}},
		"bad member":     {{Name: "g", Entries: []string{"not-an-ip"}}},
	}
	for name, groups := range cases {
		if _, err := NormalizeIPGroups(groups); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestNormalizeListRulesDefaultsDedupesAndSorts(t *testing.T) {
	groups := []IPGroup{{Name: "scanners", Entries: []string{"10.0.0.0/8"}}}
	got, err := NormalizeListRules([]ListRule{
		// No list given: an entry defaults to the blacklist, which is the
		// conservative reading of an incompletely specified rule.
		{Target: ListTargetURL, Pattern: "/wp-admin"},
		{List: ListAllow, Target: ListTargetIP, Pattern: " 203.0.113.7 "},
		{List: ListDeny, Target: ListTargetIPGroup, Pattern: "scanners"},
		// Exact duplicate of the first entry.
		{List: ListDeny, Target: ListTargetURL, Match: ListMatchContains, Pattern: "/wp-admin"},
	}, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("duplicates must collapse, got %#v", got)
	}
	// Allow entries sort before deny ones purely for determinism; the gateway
	// evaluates every deny before any allow regardless of stored order.
	if got[0].List != ListAllow {
		t.Fatalf("unexpected order: %#v", got)
	}
	for _, r := range got {
		if r.Target == ListTargetURL && r.Match != ListMatchContains {
			t.Fatalf("URL entries must default to a contains match: %#v", r)
		}
		if (r.Target == ListTargetIP || r.Target == ListTargetIPGroup) && r.Match != "" {
			t.Fatalf("address entries must carry no match type: %#v", r)
		}
	}
}

func TestNormalizeListRulesRejectsInvalid(t *testing.T) {
	groups := []IPGroup{{Name: "known", Entries: []string{"10.0.0.0/8"}}}
	cases := map[string]ListRule{
		"unknown list":      {List: "maybe", Target: ListTargetURL, Pattern: "x"},
		"unknown target":    {List: ListDeny, Target: "cookie", Pattern: "x"},
		"unknown match":     {List: ListDeny, Target: ListTargetURL, Match: "fuzzy", Pattern: "x"},
		"empty pattern":     {List: ListDeny, Target: ListTargetURL, Pattern: "   "},
		"invalid ip":        {List: ListDeny, Target: ListTargetIP, Pattern: "300.1.1.1"},
		"unknown group":     {List: ListDeny, Target: ListTargetIPGroup, Pattern: "ghosts"},
		"invalid regex":     {List: ListDeny, Target: ListTargetURL, Match: ListMatchRegex, Pattern: "a(("},
		"oversized pattern": {List: ListDeny, Target: ListTargetURL, Pattern: strings.Repeat("x", MaxListPatternBytes+1)},
	}
	for name, rule := range cases {
		if _, err := NormalizeListRules([]ListRule{rule}, groups); err == nil {
			t.Fatalf("%s must be rejected before it reaches the gateway", name)
		}
	}
}

func TestBuildAttachesListsAndAffectsGeneration(t *testing.T) {
	site := []Site{{
		Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
		Domains: []model.WebsiteDomain{{Domain: "a.example"}},
		Enabled: true,
	}}
	plain, err := Build(site)
	if err != nil {
		t.Fatal(err)
	}
	withLists, err := BuildWithOptions(site, BuildOptions{
		IPGroups: []IPGroup{{Name: "scanners", Entries: []string{"10.0.0.0/8"}}},
		Lists:    []ListRule{{List: ListDeny, Target: ListTargetIPGroup, Pattern: "scanners"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(withLists.Lists) != 1 || len(withLists.IPGroups) != 1 {
		t.Fatalf("lists must reach the emitted config: %+v", withLists)
	}
	if plain.Generation == withLists.Generation {
		t.Fatal("adding a list must change the config generation")
	}
	// A list entry pointing at a group that does not exist must fail generation
	// rather than produce a config the gateway will refuse to load.
	if _, err := BuildWithOptions(site, BuildOptions{
		Lists: []ListRule{{List: ListDeny, Target: ListTargetIPGroup, Pattern: "ghosts"}},
	}); err == nil {
		t.Fatal("a dangling IP group reference must fail config generation")
	}
}
