package service

import (
	"reflect"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
)

// The master switch off must yield NOTHING to the data plane, not a list with a
// disabled flag: the gateway never holds a rule it is not meant to apply, and a
// stored-but-unenforced rule showing up in the config would make the two
// disagree about what is in force.
func TestUploadMasterSwitchOffSendsNothing(t *testing.T) {
	rows := []model.WafUploadRule{
		{WebsiteID: 1, Rule: "php", Enabled: true},
		{WebsiteID: 1, Rule: "jsp", Enabled: true},
	}
	if got := enabledUploadRules(rows, 1, false); len(got) != 0 {
		t.Fatalf("the switch is off, nothing may reach the data plane, got %v", got)
	}
	if got := enabledUploadRules(rows, 1, true); !reflect.DeepEqual(got, []string{"php", "jsp"}) {
		t.Fatalf("the switch is on, both rules must reach the data plane, got %v", got)
	}
}

// A rule switched off individually, and any rule belonging to another site, must
// not leak into this site's policy.
func TestUploadRulesAreScopedAndRespectRowSwitches(t *testing.T) {
	rows := []model.WafUploadRule{
		{WebsiteID: 1, Rule: "php", Enabled: true},
		{WebsiteID: 1, Rule: "jsp", Enabled: false},
		{WebsiteID: 2, Rule: "exe", Enabled: true},
	}
	got := enabledUploadRules(rows, 1, true)
	if !reflect.DeepEqual(got, []string{"php"}) {
		t.Fatalf("only this site's enabled rules may be sent, got %v", got)
	}
}

// The upload list has exactly one home. Attaching it at config-generation time
// must not disturb the detection policy it rides along with, and an empty list
// must leave that policy untouched — including leaving a nil policy nil, so an
// all-default site is still emitted as absent.
func TestWithUploadRulesLeavesTheDetectionPolicyAlone(t *testing.T) {
	if got := withUploadRules(nil, nil); got != nil {
		t.Fatalf("no policy and no rules must stay absent, got %+v", got)
	}
	got := withUploadRules(nil, []string{"php"})
	if got == nil || !reflect.DeepEqual(got.UploadRules, []string{"php"}) {
		t.Fatalf("rules must reach the data plane even with no detection policy: %+v", got)
	}
	if got.DisableSQLi || got.DisableXSS || got.Strict {
		t.Fatalf("attaching upload rules must not weaken detection: %+v", got)
	}
}
