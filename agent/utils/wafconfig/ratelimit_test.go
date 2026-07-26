package wafconfig

import (
	"reflect"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
)

func TestNormalizeRateLimitsSortsAndForcesPerURL(t *testing.T) {
	got, err := NormalizeRateLimits([]RateLimit{
		{Kind: RateLimitNotFound, PeriodSec: 60, Threshold: 30, BanSec: 600},
		{Kind: RateLimitURL, PeriodSec: 10, Threshold: 5, BanSec: 600},
		{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 200, BanSec: 600},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []RateLimit{
		{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 200, BanSec: 600},
		{Kind: RateLimitNotFound, PeriodSec: 60, Threshold: 30, BanSec: 600},
		// The dedicated URL limit is per-target by definition.
		{Kind: RateLimitURL, PeriodSec: 10, Threshold: 5, BanSec: 600, PerURL: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestNormalizeRateLimitsRejectsInvalid(t *testing.T) {
	cases := map[string][]RateLimit{
		"unknown kind":    {{Kind: "nope", PeriodSec: 10, Threshold: 5}},
		"zero period":     {{Kind: RateLimitAccess, PeriodSec: 0, Threshold: 5}},
		"period too long": {{Kind: RateLimitAccess, PeriodSec: MaxRateLimitPeriodSec + 1, Threshold: 5}},
		"zero threshold":  {{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 0}},
		"negative ban":    {{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 5, BanSec: -1}},
		"ban too long":    {{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 5, BanSec: MaxRateLimitBanSec + 1}},
		"duplicate kind":  {{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 5}, {Kind: RateLimitAccess, PeriodSec: 20, Threshold: 9}},
	}
	for name, limits := range cases {
		if _, err := NormalizeRateLimits(limits); err == nil {
			t.Fatalf("%s must be rejected before it reaches the gateway", name)
		}
	}
}

func TestBuildAttachesRateLimitsAndAffectsGeneration(t *testing.T) {
	mk := func(limits []RateLimit) GatewayConfig {
		cfg, err := Build([]Site{{
			Website:    model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
			Domains:    []model.WebsiteDomain{{Domain: "a.example"}},
			Enabled:    true,
			RateLimits: limits,
		}})
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	base := mk(nil)
	limited := mk([]RateLimit{{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 200, BanSec: 600}})

	if len(limited.Sites) != 1 || len(limited.Sites[0].RateLimits) != 1 {
		t.Fatalf("limits must reach the emitted site: %+v", limited.Sites)
	}
	if base.Generation == limited.Generation {
		t.Fatal("adding a rate limit must change the config generation")
	}
	// Equivalent policies expressed in a different order must stay stable, so an
	// unchanged policy never forces a needless gateway reload.
	a := mk([]RateLimit{
		{Kind: RateLimitNotFound, PeriodSec: 60, Threshold: 30},
		{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 200},
	})
	b := mk([]RateLimit{
		{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 200},
		{Kind: RateLimitNotFound, PeriodSec: 60, Threshold: 30},
	})
	if a.Generation != b.Generation {
		t.Fatalf("limit ordering must not change the generation: %s vs %s", a.Generation, b.Generation)
	}
}

func TestBuildAttachesGatewayWideAttackLimit(t *testing.T) {
	site := []Site{{
		Website: model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
		Domains: []model.WebsiteDomain{{Domain: "a.example"}},
		Enabled: true,
	}}
	cfg, err := BuildWithOptions(site, BuildOptions{AttackRateLimit: &RateLimit{PeriodSec: 60, Threshold: 5, BanSec: 600}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AttackRateLimit == nil {
		t.Fatal("the attack limit must reach the emitted config")
	}
	// The kind defaults, and a per-URL flag is meaningless for a limit counted
	// from engine callbacks that carry no request target.
	if cfg.AttackRateLimit.Kind != RateLimitAttack || cfg.AttackRateLimit.PerURL {
		t.Fatalf("unexpected attack limit: %+v", cfg.AttackRateLimit)
	}

	plain, err := Build(site)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Generation == cfg.Generation {
		t.Fatal("a gateway-wide setting must change the generation digest")
	}

	if _, err := BuildWithOptions(site, BuildOptions{AttackRateLimit: &RateLimit{Kind: RateLimitAccess, PeriodSec: 60, Threshold: 5}}); err == nil {
		t.Fatal("the gateway attack slot must reject a limit of another kind")
	}
	if _, err := BuildWithOptions(site, BuildOptions{AttackRateLimit: &RateLimit{PeriodSec: 0, Threshold: 5}}); err == nil {
		t.Fatal("an invalid attack limit must fail config generation")
	}
}

func TestBuildRejectsInvalidRateLimit(t *testing.T) {
	_, err := Build([]Site{{
		Website:    model.Website{BaseModel: model.BaseModel{ID: 1}, Type: constant.Proxy, Alias: "alpha", Proxy: "127.0.0.1:8080"},
		Domains:    []model.WebsiteDomain{{Domain: "a.example"}},
		Enabled:    true,
		RateLimits: []RateLimit{{Kind: RateLimitAccess, PeriodSec: 10, Threshold: 0}},
	}})
	if err == nil {
		t.Fatal("an invalid limit must fail config generation, not reach the gateway")
	}
}
