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

func TestPublicEnhancementSettingExcludesWatermark(t *testing.T) {
	setupEnhancementTestDB(t)
	if err := settingRepo.UpdateOrCreate("Theme", "dark"); err != nil {
		t.Fatalf("store theme: %v", err)
	}
	if err := settingRepo.UpdateOrCreate("ThemeColor", `{"light":"#112233","dark":"#445566"}`); err != nil {
		t.Fatalf("store theme color: %v", err)
	}
	watermark := `{"lightColor":"#000000","darkColor":"#ffffff","fontSize":16,"content":"internal-node","rotate":0,"gap":100}`
	if err := settingRepo.UpdateOrCreate("Watermark", watermark); err != nil {
		t.Fatalf("store watermark: %v", err)
	}
	if err := settingRepo.UpdateOrCreate("WatermarkShow", "Enable"); err != nil {
		t.Fatalf("store watermark status: %v", err)
	}

	publicSetting, err := NewIEnhancementService().GetPublicSettingInfo()
	if err != nil {
		t.Fatalf("get public enhancement setting: %v", err)
	}
	data, err := json.Marshal(publicSetting)
	if err != nil {
		t.Fatalf("marshal public enhancement setting: %v", err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("decode public enhancement setting: %v", err)
	}
	if len(fields) != 2 || fields["theme"] != "dark" {
		t.Fatalf("unexpected public fields: %s", data)
	}
	if _, exists := fields["watermark"]; exists {
		t.Fatalf("public response exposed watermark: %s", data)
	}
	if _, exists := fields["watermarkShow"]; exists {
		t.Fatalf("public response exposed watermark status: %s", data)
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
