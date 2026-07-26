package service

import (
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
)

// A stored rule whose conditions cannot be read must FAIL config generation, not
// be skipped. Skipping a deny rule would leave the panel listing a protection
// that is not in force anywhere.
func TestUnreadableCustomRuleFailsConfigGeneration(t *testing.T) {
	_, err := enabledCustomRules([]model.WafCustomRule{
		{BaseModel: model.BaseModel{ID: 7}, WebsiteID: 1, Name: "broken", Action: "deny", Conditions: "{not json", Enabled: true},
	}, 1)
	if err == nil {
		t.Fatal("an unreadable rule must fail config generation rather than be dropped")
	}
	if !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "7") {
		t.Fatalf("the error must name the offending rule, got %v", err)
	}
}

// A rule switched off is omitted entirely rather than sent with a flag, so the
// data plane never holds a rule it is not meant to apply — and an unreadable
// DISABLED row must not block config generation for everything else.
func TestDisabledCustomRulesAreOmitted(t *testing.T) {
	got, err := enabledCustomRules([]model.WafCustomRule{
		{WebsiteID: 1, Name: "off", Action: "deny", Conditions: "{not json", Enabled: false},
		{WebsiteID: 1, Name: "on", Action: "deny", Conditions: `[{"field":"path","match":"prefix","pattern":"/admin"}]`, Enabled: true},
		// Another site's rule must not leak into this one's policy.
		{WebsiteID: 2, Name: "other-site", Action: "deny", Conditions: `[{"field":"path","pattern":"/x"}]`, Enabled: true},
	}, 1)
	if err != nil {
		t.Fatalf("a disabled unreadable row must not fail generation: %v", err)
	}
	if len(got) != 1 || got[0].Name != "on" {
		t.Fatalf("only this site's enabled rules may reach the data plane, got %+v", got)
	}
	if len(got[0].Conditions) != 1 || got[0].Conditions[0].Pattern != "/admin" {
		t.Fatalf("conditions did not round-trip: %+v", got[0].Conditions)
	}
}

// An unreadable row is shown as broken rather than as a rule with no
// conditions: "matches nothing" is a far more reassuring claim than the truth.
func TestUnreadableCustomRuleIsReportedNotHidden(t *testing.T) {
	got := customRulesToResponse([]model.WafCustomRule{
		{BaseModel: model.BaseModel{ID: 3}, WebsiteID: 1, Name: "broken", Action: "deny", Conditions: "", Enabled: true},
	})
	if len(got) != 1 {
		t.Fatalf("the row must still be listed, got %d", len(got))
	}
	if got[0].Invalid == "" {
		t.Fatal("an unreadable row must be reported as invalid")
	}
	if len(got[0].Conditions) != 0 {
		t.Fatalf("a broken row must not invent conditions: %+v", got[0].Conditions)
	}
}
