package helper

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	alertUtil "github.com/1Panel-dev/1Panel/agent/utils/alert"
	"github.com/1Panel-dev/1Panel/agent/utils/webhook_alert"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack/providers"
)

type alertHelper struct{}

func NewIAlertProvider() providers.AlertProvider {
	return &alertHelper{}
}

func (a *alertHelper) CreateTaskScanSMSAlertLog(alert dto.AlertDTO, alertType string, create dto.AlertLogCreate, pushAlert dto.PushAlert, config model.AlertConfig, method string) error {
	return nil
}

func (a *alertHelper) CreateSMSAlertLog(alertType string, info dto.AlertDTO, create dto.AlertLogCreate, project string, params []dto.Param, config model.AlertConfig, method string) error {
	return nil
}

func (a *alertHelper) CreateTaskScanWebhookAlertLog(alert dto.AlertDTO, alertType string, create dto.AlertLogCreate, pushAlert dto.PushAlert, config model.AlertConfig, transport *http.Transport, agentInfo *dto.AgentInfo) error {
	params := alertUtil.CreateAlertParams(alertUtil.GetCronJobTypeName(pushAlert.Param))
	return a.createWebhookAlertLog(alertType, alert, create, pushAlert.TaskName, params, config, transport, agentInfo)
}

func (a *alertHelper) CreateWebhookAlertLog(alertType string, info dto.AlertDTO, create dto.AlertLogCreate, project string, params []dto.Param, config model.AlertConfig, transport *http.Transport, agentInfo *dto.AgentInfo) error {
	return a.createWebhookAlertLog(alertType, info, create, project, params, config, transport, agentInfo)
}

func (a *alertHelper) createWebhookAlertLog(alertType string, info dto.AlertDTO, create dto.AlertLogCreate, project string, params []dto.Param, config model.AlertConfig, transport *http.Transport, agentInfo *dto.AgentInfo) error {
	create.AlertRule = alertUtil.ProcessAlertRule(info)
	create.AlertDetail = alertUtil.ProcessAlertDetail(info, project, params, config.Type)

	var webhookConfig dto.AlertWebhookConfig
	deliveryErr := json.Unmarshal([]byte(config.Config), &webhookConfig)
	if deliveryErr == nil {
		webhookConfig.Url, deliveryErr = webhook_alert.DecryptWebhookURL(webhookConfig.Url)
	}
	if deliveryErr == nil {
		content := alertUtil.GetSendContent(alertType, params, agentInfo)
		if content == "" {
			content = i18n.GetMsgWithMap("CommonAlert", map[string]interface{}{"msg": info.Title})
		}
		if title := strings.TrimSpace(i18n.GetMsgByKey("PanelAlertTitle")); title != "" {
			content = title + "\n" + content
		}
		deliveryErr = webhook_alert.Send(context.Background(), webhook_alert.Platform(config.Type), webhookConfig.Url, content, transport)
	}

	if deliveryErr != nil {
		create.Status = constant.AlertError
		create.Message = truncateAlertMessage(deliveryErr.Error())
	} else {
		create.Status = constant.AlertSuccess
		create.Message = ""
	}

	var alertLog model.AlertLog
	return alertUtil.SaveAlertLog(create, &alertLog)
}

func truncateAlertMessage(message string) string {
	const maxLength = 256
	runes := []rune(message)
	if len(runes) <= maxLength {
		return message
	}
	return string(runes[:maxLength])
}

func (a *alertHelper) GetLicenseErrorAlert() (uint, error) {
	return 0, nil
}

func (a *alertHelper) GetNodeErrorAlert() (uint, error) {
	return 0, nil
}
