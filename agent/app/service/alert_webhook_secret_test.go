package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/webhook_alert"
)

func setAlertTestEncryptKey(t *testing.T) {
	t.Helper()
	global.CONF.Base.EncryptKey = "unit-test-key-0123456789"
}

func TestEncryptWebhookAlertConfigURL(t *testing.T) {
	setAlertTestEncryptKey(t)
	plainURL := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=top-secret"
	req := dto.AlertConfigUpdate{
		Type:   constant.WeCom,
		Config: `{"displayName":"ops","url":"` + plainURL + `"}`,
	}
	if err := encryptWebhookAlertConfigURL(&req); err != nil {
		t.Fatalf("encrypt config: %v", err)
	}
	var cfg dto.AlertWebhookConfig
	if err := json.Unmarshal([]byte(req.Config), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !webhook_alert.IsEncryptedWebhookURL(cfg.Url) {
		t.Fatalf("url not encrypted: %q", cfg.Url)
	}
	if strings.Contains(req.Config, "top-secret") {
		t.Fatalf("plaintext secret survived in stored config: %q", req.Config)
	}
	if cfg.DisplayName != "ops" {
		t.Fatalf("display name changed: %q", cfg.DisplayName)
	}
	back, err := webhook_alert.DecryptWebhookURL(cfg.Url)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if back != plainURL {
		t.Fatalf("round-trip mismatch: %q vs %q", back, plainURL)
	}
}

func TestEncryptWebhookAlertConfigURLSkipsNonWebhook(t *testing.T) {
	setAlertTestEncryptKey(t)
	original := `{"password":"secret"}`
	req := dto.AlertConfigUpdate{Type: constant.Email, Config: original}
	if err := encryptWebhookAlertConfigURL(&req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Config != original {
		t.Fatalf("non-webhook config was modified: %q", req.Config)
	}
}

func TestEncryptedWebhookConfigStillMasks(t *testing.T) {
	setAlertTestEncryptKey(t)
	req := dto.AlertConfigUpdate{
		Type:   constant.DingTalk,
		Config: `{"displayName":"ops","url":"https://oapi.dingtalk.com/robot/send?access_token=abc"}`,
	}
	if err := encryptWebhookAlertConfigURL(&req); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	masked := maskWebhookAlertConfigs([]model.AlertConfig{{Type: constant.DingTalk, Config: req.Config}})
	var out dto.AlertWebhookConfig
	if err := json.Unmarshal([]byte(masked[0].Config), &out); err != nil {
		t.Fatalf("decode masked: %v", err)
	}
	if out.Url != maskedWebhookURL {
		t.Fatalf("expected masked url, got %q", out.Url)
	}
	if strings.Contains(masked[0].Config, "access_token") || strings.Contains(masked[0].Config, "enc:v1:") {
		t.Fatalf("masked output leaked material: %q", masked[0].Config)
	}
}

// TestReplaceMaskedWebhookURLDecryptsStored covers the edit-preserve path: when
// a user re-saves an existing webhook config with a masked URL, the stored
// ciphertext must be decrypted back to plaintext so re-validation and
// re-encryption operate on the real URL.
func TestReplaceMaskedWebhookURLDecryptsStored(t *testing.T) {
	setAlertTestEncryptKey(t)
	plainURL := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=restore-me"
	enc, err := webhook_alert.EncryptWebhookURL(plainURL)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	storedConfig := `{"displayName":"ops","url":"` + enc + `"}`
	incoming := dto.AlertWebhookConfig{DisplayName: "renamed", Url: maskedWebhookURL}
	if err := replaceMaskedWebhookURL(&incoming, storedConfig); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if incoming.Url != plainURL {
		t.Fatalf("stored secret not decrypted on restore: %q", incoming.Url)
	}
}
