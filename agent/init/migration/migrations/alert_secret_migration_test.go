package migrations

import (
	"encoding/json"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/webhook_alert"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestEncryptWebhookAlertConfigURLMigration verifies the at-rest migration
// encrypts existing plaintext webhook rows exactly once, leaves non-webhook
// rows untouched, and is idempotent on re-run.
func TestEncryptWebhookAlertConfigURLMigration(t *testing.T) {
	oldAlertDB := global.AlertDB
	oldKey := global.CONF.Base.EncryptKey
	global.CONF.Base.EncryptKey = "unit-test-key-0123456789"

	db, err := gorm.Open(sqlite.Open("file:migrate-webhook-secret?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AlertConfig{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	global.AlertDB = db

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		global.AlertDB = oldAlertDB
		global.CONF.Base.EncryptKey = oldKey
	})

	plainURL := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=migrate-me"
	webhookRow := model.AlertConfig{Type: constant.WeCom, Title: "w", Status: "Enable", Config: `{"displayName":"ops","url":"` + plainURL + `"}`}
	emailRow := model.AlertConfig{Type: constant.Email, Title: "e", Status: "Enable", Config: `{"password":"secret"}`}
	if err := db.Create(&webhookRow).Error; err != nil {
		t.Fatalf("seed webhook row: %v", err)
	}
	if err := db.Create(&emailRow).Error; err != nil {
		t.Fatalf("seed email row: %v", err)
	}

	if err := EncryptWebhookAlertConfigURL.Migrate(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	reloadWebhook := func() model.AlertConfig {
		var got model.AlertConfig
		if err := db.First(&got, webhookRow.ID).Error; err != nil {
			t.Fatalf("reload webhook row: %v", err)
		}
		return got
	}

	afterFirst := reloadWebhook()
	var cfg dto.AlertWebhookConfig
	if err := json.Unmarshal([]byte(afterFirst.Config), &cfg); err != nil {
		t.Fatalf("decode migrated config: %v", err)
	}
	if !webhook_alert.IsEncryptedWebhookURL(cfg.Url) {
		t.Fatalf("url not encrypted after migration: %q", cfg.Url)
	}
	back, err := webhook_alert.DecryptWebhookURL(cfg.Url)
	if err != nil {
		t.Fatalf("decrypt migrated url: %v", err)
	}
	if back != plainURL {
		t.Fatalf("migrated url decrypt mismatch: got %q want %q", back, plainURL)
	}

	// Idempotent: a second pass must not double-encrypt or otherwise change it.
	if err := EncryptWebhookAlertConfigURL.Migrate(db); err != nil {
		t.Fatalf("migration rerun: %v", err)
	}
	afterSecond := reloadWebhook()
	if afterFirst.Config != afterSecond.Config {
		t.Fatalf("migration not idempotent:\n first:  %q\n second: %q", afterFirst.Config, afterSecond.Config)
	}

	// Non-webhook rows are untouched.
	var gotEmail model.AlertConfig
	if err := db.First(&gotEmail, emailRow.ID).Error; err != nil {
		t.Fatalf("reload email row: %v", err)
	}
	if gotEmail.Config != `{"password":"secret"}` {
		t.Fatalf("non-webhook row modified: %q", gotEmail.Config)
	}
}
