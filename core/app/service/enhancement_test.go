package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupEnhancementTestDB(t *testing.T) {
	t.Helper()
	oldDB := global.DB
	dbName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dbName)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open enhancement test database: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate enhancement test database: %v", err)
	}
	global.DB = db
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		global.DB = oldDB
	})
}

func TestValidateEnhancementThemeColor(t *testing.T) {
	valid := `{"light":"#005eeb","dark":"rgba(255, 255, 255, 0.15)","themePredefineColors":{"light":["#238636"],"dark":["#F0BE96"]}}`
	if err := validateEnhancementSetting("ThemeColor", valid); err != nil {
		t.Fatalf("expected valid theme colors, got %v", err)
	}

	invalid := `{"light":"red; background:url(javascript:alert(1))","dark":"#000000"}`
	if err := validateEnhancementSetting("ThemeColor", invalid); err == nil {
		t.Fatal("expected unsafe theme color to be rejected")
	}
}

func TestValidateEnhancementWatermark(t *testing.T) {
	valid := `{"lightColor":"rgba(0, 0, 0, 0.15)","darkColor":"#ffffff","fontSize":16,"content":"${nodeName} - ${nodeAddr}","rotate":-22,"gap":100}`
	if err := validateEnhancementSetting("Watermark", valid); err != nil {
		t.Fatalf("expected valid watermark, got %v", err)
	}

	invalid := `{"lightColor":"#000000","darkColor":"#ffffff","fontSize":101,"content":"test","rotate":0,"gap":100}`
	if err := validateEnhancementSetting("Watermark", invalid); err == nil {
		t.Fatal("expected invalid watermark font size to be rejected")
	}
}

func TestValidateEnhancementSettingRejectsUnknownKey(t *testing.T) {
	if err := validateEnhancementSetting("License", "Bound"); err == nil {
		t.Fatal("expected unknown enhancement key to be rejected")
	}
}

func TestDefaultEnhancementValuesAreValid(t *testing.T) {
	tests := map[string]string{
		"Theme":         defaultEnhancementTheme,
		"ThemeColor":    defaultEnhancementThemeColor,
		"Watermark":     defaultEnhancementWatermark,
		"WatermarkShow": "Disable",
	}
	for key, value := range tests {
		if err := validateEnhancementSetting(key, value); err != nil {
			t.Fatalf("default %s setting is invalid: %v", key, err)
		}
	}
}

// TestPublicEnhancementSettingIsStrictSubset is the anonymous-surface control
// (threat T8): the pre-auth payload exposes exactly the cosmetic set and never
// watermark, image bytes/paths, or any key absent from the authenticated DTO.
func TestPublicEnhancementSettingIsStrictSubset(t *testing.T) {
	setupEnhancementTestDB(t)
	stored := map[string]string{
		"Theme":             "dark",
		"ThemeColor":        `{"light":"#112233","dark":"#445566"}`,
		"Watermark":         `{"lightColor":"#000000","darkColor":"#ffffff","fontSize":16,"content":"internal-node","rotate":0,"gap":100}`,
		"WatermarkShow":     "Enable",
		"Title":             "Acme Cloud",
		"MasterAlias":       "HQ",
		"LoginBgType":       "color",
		"LoginBackground":   "#101828",
		"LoginBtnLinkColor": "rgb(0, 94, 235)",
		"Logo":              "logo",
		"LogoWithText":      "logoWithText",
		"Favicon":           "favicon",
		"LoginImage":        "loginImage",
	}
	for k, v := range stored {
		if err := settingRepo.UpdateOrCreate(k, v); err != nil {
			t.Fatalf("store %s: %v", k, err)
		}
	}

	svc := NewIEnhancementService()
	publicSetting, err := svc.GetPublicSettingInfo()
	if err != nil {
		t.Fatalf("get public enhancement setting: %v", err)
	}
	authedSetting, err := svc.GetSettingInfo()
	if err != nil {
		t.Fatalf("get authed enhancement setting: %v", err)
	}

	publicFields := jsonKeySet(t, publicSetting)
	authedFields := jsonKeySet(t, authedSetting)

	// The public surface exposes cosmetic text/colors plus presence sentinels
	// for images that are ALREADY anonymously served at /api/v2/images/* — so a
	// sentinel discloses nothing new. It must still never carry the watermark,
	// bytes, paths, or any key absent from the authenticated DTO (T8).
	wantPublic := map[string]bool{
		"theme": true, "themeColor": true, "title": true, "masterAlias": true,
		"loginBgType": true, "loginBackground": true, "loginBtnLinkColor": true,
		"logo": true, "logoWithText": true, "favicon": true, "loginImage": true,
	}
	for k := range publicFields {
		if !wantPublic[k] {
			t.Fatalf("public payload exposed unexpected key %q", k)
		}
		if _, ok := authedFields[k]; !ok {
			t.Fatalf("public key %q is not a subset of the authenticated DTO", k)
		}
	}
	for k := range wantPublic {
		if _, ok := publicFields[k]; !ok {
			t.Fatalf("public payload missing expected cosmetic key %q", k)
		}
	}
	for _, forbidden := range []string{"watermark", "watermarkShow"} {
		if _, exists := publicFields[forbidden]; exists {
			t.Fatalf("public response exposed forbidden key %q", forbidden)
		}
	}
	// The image fields are presence sentinels only — never a path or bytes.
	for k, v := range map[string]string{
		"logo": publicSetting.Logo, "logoWithText": publicSetting.LogoWithText,
		"favicon": publicSetting.Favicon, "loginImage": publicSetting.LoginImage,
	} {
		if v != k {
			t.Fatalf("public image field %q must be the presence sentinel %q, got %q", k, k, v)
		}
	}
	if publicSetting.Title != "Acme Cloud" || publicSetting.LoginBackground != "#101828" {
		t.Fatalf("public branding values not returned: %+v", publicSetting)
	}
}

func jsonKeySet(t *testing.T, v interface{}) map[string]struct{} {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	set := make(map[string]struct{}, len(fields))
	for k := range fields {
		set[k] = struct{}{}
	}
	return set
}

func TestValidateEnhancementBrandingFields(t *testing.T) {
	valid := []struct{ key, value string }{
		{"Title", ""}, {"Title", "Acme Cloud 面板"},
		{"MasterAlias", "HQ-Node"},
		{"LoginBgType", ""}, {"LoginBgType", "image"}, {"LoginBgType", "color"},
		{"LoginBackground", ""}, {"LoginBackground", "#0a0a0a"}, {"LoginBackground", "rgba(0,0,0,0.5)"},
		{"LoginBackground", "loginBackground"}, // image-mode presence sentinel
		{"LoginBtnLinkColor", "#005eeb"},
		// Image presence sentinels: empty (unset) or the exact camel-case key.
		{"Logo", ""}, {"Logo", "logo"},
		{"LogoWithText", ""}, {"LogoWithText", "logoWithText"},
		{"Favicon", ""}, {"Favicon", "favicon"},
		{"LoginImage", ""}, {"LoginImage", "loginImage"},
	}
	for _, c := range valid {
		if err := validateEnhancementSetting(c.key, c.value); err != nil {
			t.Fatalf("expected %s=%q valid, got %v", c.key, c.value, err)
		}
	}

	invalid := []struct {
		name, key, value string
	}{
		{"title angle bracket", "Title", "<img src=x onerror=alert(1)>"},
		{"title control char", "Title", "brand\x00name"},
		{"title too long", "Title", strings.Repeat("a", 65)},
		{"alias angle bracket", "MasterAlias", "a<b"},
		{"bgtype bad enum", "LoginBgType", "video"},
		{"background not color", "LoginBackground", "javascript:alert(1)"},
		{"background bad word", "LoginBackground", "red"},
		{"btncolor not color", "LoginBtnLinkColor", "url(x)"},
		// An image field must never hold anything but its own sentinel — no
		// markup, no path, no cross-asset value.
		{"logo markup", "Logo", "<svg onload=alert(1)>"},
		{"logo path", "Logo", "/api/v2/images/logo"},
		{"logo wrong sentinel", "Logo", "favicon"},
		{"favicon wrong sentinel", "Favicon", "logo"},
		{"loginImage arbitrary", "LoginImage", "https://evil.example/x.png"},
	}
	for _, c := range invalid {
		if err := validateEnhancementSetting(c.key, c.value); err == nil {
			t.Fatalf("%s: expected %s=%q to be rejected", c.name, c.key, c.value)
		}
	}
}

func TestEnhancementBrandingStoredValueFallsBack(t *testing.T) {
	setupEnhancementTestDB(t)
	// A corrupt stored branding value must fall back to empty, never surface markup.
	if err := settingRepo.UpdateOrCreate("Title", "<script>evil</script>"); err != nil {
		t.Fatalf("store corrupt title: %v", err)
	}
	if err := settingRepo.UpdateOrCreate("LoginBackground", "expression(alert(1))"); err != nil {
		t.Fatalf("store corrupt background: %v", err)
	}
	info, err := NewIEnhancementService().GetSettingInfo()
	if err != nil {
		t.Fatalf("get setting info: %v", err)
	}
	if info.Title != "" {
		t.Fatalf("corrupt title did not fall back: %q", info.Title)
	}
	if info.LoginBackground != "" {
		t.Fatalf("corrupt background did not fall back: %q", info.LoginBackground)
	}
}

func TestEnhancementStoredValuesFallBackAndLegacyThemeUpdateIsValidated(t *testing.T) {
	setupEnhancementTestDB(t)
	if err := settingRepo.UpdateOrCreate("Theme", "invalid-theme"); err != nil {
		t.Fatalf("store invalid theme: %v", err)
	}
	if err := settingRepo.UpdateOrCreate("ThemeColor", `{"light":"url(javascript:bad)","dark":"#000000"}`); err != nil {
		t.Fatalf("store invalid theme color: %v", err)
	}

	publicSetting, err := NewIEnhancementService().GetPublicSettingInfo()
	if err != nil {
		t.Fatalf("get public enhancement setting: %v", err)
	}
	if publicSetting.Theme != defaultEnhancementTheme || publicSetting.ThemeColor != defaultEnhancementThemeColor {
		t.Fatalf("invalid stored values did not fall back: %+v", publicSetting)
	}

	if err := (&SettingService{}).Update(nil, "Theme", "still-invalid"); err == nil {
		t.Fatal("expected legacy Theme update path to apply enhancement validation")
	}
}
