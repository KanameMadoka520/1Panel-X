package service

import (
	"encoding/json"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
)

func TestValidateWebhookAlertConfig(t *testing.T) {
	valid := `{"displayName":"operations","url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test"}`
	if err := validateWebhookAlertConfig(constant.WeCom, valid); err != nil {
		t.Fatalf("expected valid webhook config, got %v", err)
	}

	tests := []struct {
		name       string
		configType string
		config     string
	}{
		{name: "malformed", configType: constant.WeCom, config: `{`},
		{name: "missing name", configType: constant.WeCom, config: `{"url":"https://qyapi.weixin.qq.com/hook"}`},
		{name: "http", configType: constant.WeCom, config: `{"displayName":"ops","url":"http://qyapi.weixin.qq.com/hook"}`},
		{name: "wrong platform", configType: constant.WeCom, config: `{"displayName":"ops","url":"https://oapi.dingtalk.com/robot/send"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateWebhookAlertConfig(tt.configType, tt.config); err == nil {
				t.Fatal("expected invalid webhook config to be rejected")
			}
		})
	}
}

func TestValidateWebhookAlertConfigIgnoresOtherTypes(t *testing.T) {
	if err := validateWebhookAlertConfig(constant.Email, "not-json"); err != nil {
		t.Fatalf("expected non-webhook config to be handled elsewhere, got %v", err)
	}
}

func TestMaskWebhookAlertConfigs(t *testing.T) {
	emailConfig := `{"password":"unchanged"}`
	configs := []model.AlertConfig{
		{Type: constant.WeCom, Config: `{"displayName":"operations","url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret"}`},
		{Type: constant.Email, Config: emailConfig},
	}

	masked := maskWebhookAlertConfigs(configs)
	var webhook dto.AlertWebhookConfig
	if err := json.Unmarshal([]byte(masked[0].Config), &webhook); err != nil {
		t.Fatalf("decode masked config: %v", err)
	}
	if webhook.DisplayName != "operations" || webhook.Url != maskedWebhookURL {
		t.Fatalf("unexpected masked webhook config: %+v", webhook)
	}
	if masked[1].Config != emailConfig {
		t.Fatal("expected non-webhook config to remain unchanged")
	}
}

func TestReplaceMaskedWebhookURL(t *testing.T) {
	incoming := dto.AlertWebhookConfig{DisplayName: "renamed", Url: maskedWebhookURL}
	stored := `{"displayName":"operations","url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret"}`
	if err := replaceMaskedWebhookURL(&incoming, stored); err != nil {
		t.Fatalf("replaceMaskedWebhookURL() error = %v", err)
	}
	if incoming.DisplayName != "renamed" {
		t.Fatalf("display name = %q, want renamed", incoming.DisplayName)
	}
	if incoming.Url != "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret" {
		t.Fatalf("URL was not restored from stored config: %q", incoming.Url)
	}
}
