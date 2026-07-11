package helper_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func TestFailedWebhookDeliveryCountsAsAttempt(t *testing.T) {
	oldDB := global.DB
	oldAlertDB := global.AlertDB
	oldLog := global.LOG
	oldI18n := global.I18n
	oldMultiNodeProvider := xpack.MultiNodeProvider

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	global.DB = nil
	global.LOG = logger
	i18n.Init()

	alertDB, err := gorm.Open(sqlite.Open("file:webhook-retry-accounting?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open alert database: %v", err)
	}
	if err := alertDB.AutoMigrate(&model.AlertConfig{}, &model.AlertLog{}, &model.AlertTask{}); err != nil {
		t.Fatalf("migrate alert database: %v", err)
	}
	global.AlertDB = alertDB

	dialAttempts := 0
	xpack.MultiNodeProvider = &failingWebhookMultiNodeProvider{
		transport: &http.Transport{
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				dialAttempts++
				return nil, errors.New("simulated webhook delivery failure")
			},
		},
	}

	t.Cleanup(func() {
		if sqlDB, err := alertDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		global.DB = oldDB
		global.AlertDB = oldAlertDB
		global.LOG = oldLog
		global.I18n = oldI18n
		xpack.MultiNodeProvider = oldMultiNodeProvider
	})

	const webhookSecret = "retry-accounting-secret"
	config := model.AlertConfig{
		Type:   constant.WeCom,
		Status: constant.AlertEnable,
		Config: `{"displayName":"operations","url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=` + webhookSecret + `"}`,
	}
	if err := alertDB.Create(&config).Error; err != nil {
		t.Fatalf("create webhook config: %v", err)
	}

	alert := dto.AlertDTO{
		ID:        42,
		Type:      "cpu",
		Title:     "CPU threshold exceeded",
		Method:    strconv.Itoa(int(config.ID)),
		SendCount: 5,
	}
	sender := service.NewAlertSender(alert, "node-1")
	params := []dto.Param{
		{Index: "1", Value: "2026-07-10 20:00:00"},
		{Index: "2", Value: "CPU"},
		{Index: "3", Value: "95%"},
	}

	sender.Send("node-1", params)
	sender.Send("node-1", params)

	if dialAttempts != 1 {
		t.Fatalf("webhook delivery attempts = %d, want 1", dialAttempts)
	}

	var logs []model.AlertLog
	if err := alertDB.Find(&logs).Error; err != nil {
		t.Fatalf("load alert logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("alert log count = %d, want 1", len(logs))
	}
	if logs[0].Status != constant.AlertError {
		t.Fatalf("alert log status = %q, want %q", logs[0].Status, constant.AlertError)
	}
	if logs[0].Count != 1 {
		t.Fatalf("alert log attempt count = %d, want 1", logs[0].Count)
	}
	if logs[0].Method != strconv.Itoa(int(config.ID)) {
		t.Fatalf("alert log method = %q, want config ID %d", logs[0].Method, config.ID)
	}
	if !strings.Contains(logs[0].Message, "simulated webhook delivery failure") {
		t.Fatalf("alert log message = %q, want simulated delivery error", logs[0].Message)
	}
	if strings.Contains(logs[0].Message, webhookSecret) {
		t.Fatalf("alert log message leaked webhook secret: %q", logs[0].Message)
	}

	var tasks []model.AlertTask
	if err := alertDB.Find(&tasks).Error; err != nil {
		t.Fatalf("load alert tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("alert task count = %d, want 1", len(tasks))
	}
	if tasks[0].Type != alert.Type || tasks[0].QuotaType != "node-1" || tasks[0].Method != strconv.Itoa(int(config.ID)) {
		t.Fatalf("unexpected alert task: %+v", tasks[0])
	}
}

type failingWebhookMultiNodeProvider struct {
	transport *http.Transport
}

func (p *failingWebhookMultiNodeProvider) IsXpack() bool { return false }

func (p *failingWebhookMultiNodeProvider) IsUseCustomApp() bool { return false }

func (p *failingWebhookMultiNodeProvider) GetImagePrefix() string { return "" }

func (p *failingWebhookMultiNodeProvider) RemoveTamper(string) {}

func (p *failingWebhookMultiNodeProvider) StartClam(*model.Clam, bool) (int, error) { return 0, nil }

func (p *failingWebhookMultiNodeProvider) LoadNodeInfo(bool) (model.NodeInfo, error) {
	return model.NodeInfo{}, nil
}

func (p *failingWebhookMultiNodeProvider) LoadRequestTransport() *http.Transport {
	return p.transport
}

func (p *failingWebhookMultiNodeProvider) ValidateCertificate(*gin.Context) bool { return true }

func (p *failingWebhookMultiNodeProvider) PushSSLToNode(*model.WebsiteSSL) error { return nil }

func (p *failingWebhookMultiNodeProvider) GetAgentInfo() (*dto.AgentInfo, error) {
	return &dto.AgentInfo{NodeName: "test-node", NodeAddr: "192.0.2.10"}, nil
}
