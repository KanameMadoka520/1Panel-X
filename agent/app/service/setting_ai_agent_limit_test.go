package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSettingServiceTestDB(t *testing.T) {
	t.Helper()

	oldDB := global.DB
	dsn := fmt.Sprintf("file:setting-ai-agent-limit-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open setting test database: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate setting test database: %v", err)
	}
	global.DB = db

	t.Cleanup(func() {
		global.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestSettingServiceUpdateAIAgentLimitPersistsNormalizedValues(t *testing.T) {
	setupSettingServiceTestDB(t)
	service := &SettingService{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "unlimited", input: " 0 ", expected: "0"},
		{name: "minimum positive", input: "0001", expected: "1"},
		{name: "maximum", input: "1000", expected: "1000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := service.Update("AIAgentLimit", tt.input); err != nil {
				t.Fatalf("update AIAgentLimit: %v", err)
			}

			var setting model.Setting
			if err := global.DB.Where("key = ?", "AIAgentLimit").First(&setting).Error; err != nil {
				t.Fatalf("load persisted AIAgentLimit: %v", err)
			}
			if setting.Value != tt.expected {
				t.Fatalf("expected persisted value %q, got %q", tt.expected, setting.Value)
			}
		})
	}
}

func TestSettingServiceUpdateAIAgentLimitRejectsInvalidValuesWithoutPersistence(t *testing.T) {
	setupSettingServiceTestDB(t)
	service := &SettingService{}

	if err := global.DB.Create(&model.Setting{Key: "AIAgentLimit", Value: "7"}).Error; err != nil {
		t.Fatalf("seed AIAgentLimit: %v", err)
	}

	invalidValues := []string{"-1", "1001", "1.5", "abc", "", " ", "999999999999999999999999"}
	for _, input := range invalidValues {
		t.Run(fmt.Sprintf("value_%q", input), func(t *testing.T) {
			if err := service.Update("AIAgentLimit", input); err == nil {
				t.Fatalf("expected invalid value %q to be rejected", input)
			}

			var setting model.Setting
			if err := global.DB.Where("key = ?", "AIAgentLimit").First(&setting).Error; err != nil {
				t.Fatalf("load AIAgentLimit after rejection: %v", err)
			}
			if setting.Value != "7" {
				t.Fatalf("invalid value %q changed persisted setting to %q", input, setting.Value)
			}
		})
	}
}
