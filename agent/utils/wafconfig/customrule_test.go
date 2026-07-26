package wafconfig

import (
	"reflect"
	"testing"
)

func TestNormalizeCustomRulesCanonicalizesWithoutReordering(t *testing.T) {
	in := []CustomRule{
		{Name: " watch ", Action: CustomActionLog, Conditions: []CustomCondition{
			{Field: CustomFieldPath, Pattern: " /api "},
		}},
		{Name: "block", Action: CustomActionDeny, Conditions: []CustomCondition{
			{Field: CustomFieldIP, Pattern: "203.0.113.0/24", Negate: true},
			{Field: CustomFieldHeader, Name: " X-Tenant ", Match: ListMatchExact, Pattern: "acme"},
		}},
	}
	got, err := NormalizeCustomRules(in)
	if err != nil {
		t.Fatal(err)
	}
	// Order is the operator's policy: the data plane resolves the FIRST match, so
	// canonicalization must never sort these the way it sorts the lists.
	if got[0].Name != "watch" || got[1].Name != "block" {
		t.Fatalf("rule order must be preserved: %+v", got)
	}
	// An omitted match type defaults to "contains" rather than staying empty, so
	// what is stored is what will be enforced.
	if got[0].Conditions[0].Match != ListMatchContains || got[0].Conditions[0].Pattern != "/api" {
		t.Fatalf("condition was not canonicalized: %+v", got[0].Conditions[0])
	}
	if got[1].Conditions[1].Name != "X-Tenant" {
		t.Fatalf("header name was not trimmed: %+v", got[1].Conditions[1])
	}
	if !got[1].Conditions[0].Negate {
		t.Fatal("negation must survive normalization")
	}
}

func TestNormalizeCustomRulesRejectsUnusableRules(t *testing.T) {
	bad := map[string]CustomRule{
		"no conditions":  {Action: CustomActionDeny},
		"unknown action": {Action: "drop", Conditions: []CustomCondition{{Field: CustomFieldPath, Pattern: "/x"}}},
		"unknown field":  {Action: CustomActionDeny, Conditions: []CustomCondition{{Field: "body", Pattern: "/x"}}},
		"unknown match":  {Action: CustomActionDeny, Conditions: []CustomCondition{{Field: CustomFieldPath, Match: "glob", Pattern: "/x"}}},
		"empty pattern":  {Action: CustomActionDeny, Conditions: []CustomCondition{{Field: CustomFieldPath, Pattern: "  "}}},
		"bad regex":      {Action: CustomActionDeny, Conditions: []CustomCondition{{Field: CustomFieldPath, Match: ListMatchRegex, Pattern: "("}}},
		"header no name": {Action: CustomActionDeny, Conditions: []CustomCondition{{Field: CustomFieldHeader, Pattern: "x"}}},
		"bad header name": {Action: CustomActionDeny,
			Conditions: []CustomCondition{{Field: CustomFieldHeader, Name: "X Tenant: evil", Pattern: "x"}}},
		"name where none belongs": {Action: CustomActionDeny,
			Conditions: []CustomCondition{{Field: CustomFieldPath, Name: "nope", Pattern: "/x"}}},
		"match on an address": {Action: CustomActionDeny,
			Conditions: []CustomCondition{{Field: CustomFieldIP, Match: ListMatchPrefix, Pattern: "10.0.0.0/8"}}},
		"bad address": {Action: CustomActionDeny,
			Conditions: []CustomCondition{{Field: CustomFieldIP, Pattern: "not-an-address"}}},
	}
	for name, rule := range bad {
		if _, err := NormalizeCustomRules([]CustomRule{rule}); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
	if _, err := NormalizeCustomRules(make([]CustomRule, MaxCustomRules+1)); err == nil {
		t.Fatal("an oversized rule list must be rejected")
	}
}

// Reordering rules changes which one decides a request, so it has to change the
// config generation — otherwise the gateway would keep the old order while the
// panel showed the new one.
func TestCustomRuleOrderAffectsGeneration(t *testing.T) {
	deny := CustomRule{Action: CustomActionDeny, Conditions: []CustomCondition{{Field: CustomFieldPath, Pattern: "/a"}}}
	allow := CustomRule{Action: CustomActionAllow, Conditions: []CustomCondition{{Field: CustomFieldPath, Pattern: "/a"}}}

	first, err := BuildWithOptions(nil, BuildOptions{CustomRules: []CustomRule{deny, allow}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildWithOptions(nil, BuildOptions{CustomRules: []CustomRule{allow, deny}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation == second.Generation {
		t.Fatal("swapping two rules must change the config generation")
	}
	if !reflect.DeepEqual(first.CustomRules[0].Action, CustomActionDeny) {
		t.Fatalf("the emitted order must match the input: %+v", first.CustomRules)
	}
	// An unchanged policy must not churn the generation, or every unrelated save
	// would force a needless gateway reload.
	again, err := BuildWithOptions(nil, BuildOptions{CustomRules: []CustomRule{deny, allow}})
	if err != nil {
		t.Fatal(err)
	}
	if again.Generation != first.Generation {
		t.Fatal("an unchanged rule list must produce a stable generation")
	}
}

func TestBuildRejectsInvalidCustomRule(t *testing.T) {
	if _, err := BuildWithOptions(nil, BuildOptions{
		CustomRules: []CustomRule{{Action: CustomActionDeny}},
	}); err == nil {
		t.Fatal("a rule with no conditions must fail config generation")
	}
}
