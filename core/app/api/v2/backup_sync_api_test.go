package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/service"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type backupSyncAPIService struct {
	service.IBackupService
	status dto.BackupSyncStatus
}

func (f *backupSyncAPIService) Create(dto.BackupOperate) error {
	return nil
}

func (f *backupSyncAPIService) GetSyncStatus(string) (dto.BackupSyncStatus, error) {
	return f.status, nil
}

func (f *backupSyncAPIService) ListSyncStatuses() ([]dto.BackupSyncStatus, error) {
	return []dto.BackupSyncStatus{f.status}, nil
}

func (f *backupSyncAPIService) RetrySync(string) (dto.BackupSyncStatus, error) {
	return f.status, nil
}

func TestCreateBackupReturnsAppliedWithPartialSyncStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldService := backupService
	oldValidator := global.VALID
	global.VALID = validator.New()
	backupService = &backupSyncAPIService{status: dto.BackupSyncStatus{
		AccountName: "shared",
		Revision:    9,
		Status:      "partially_synced",
		Succeeded:   1,
		Pending:     1,
		Total:       2,
		Targets: []dto.BackupSyncTargetStatus{
			{TargetKey: "local", NodeName: "local", Status: "synced", AppliedRevision: 9, DesiredRevision: 9},
			{TargetKey: "node:2", NodeID: 2, NodeName: "remote", Status: "failed", AppliedRevision: 8, DesiredRevision: 9, LastError: "connection unavailable"},
		},
	}}
	t.Cleanup(func() {
		backupService = oldService
		global.VALID = oldValidator
	})

	router := gin.New()
	router.POST("/core/backups", ApiGroupApp.BaseApi.CreateBackup)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/core/backups",
		strings.NewReader(`{"name":"shared","type":"S3","isPublic":true,"vars":"{}"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected create status=%d body=%s", recorder.Code, body)
	}
	for _, expected := range []string{`"applied":true`, `"status":"partially_synced"`, `"pending":1`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("create response does not contain %s: %s", expected, body)
		}
	}
	for _, forbidden := range browserForbiddenBackupSyncFields() {
		if strings.Contains(body, forbidden) {
			t.Fatalf("create response contains forbidden field/value %q: %s", forbidden, body)
		}
	}
}

func TestRetryBackupSyncReturnsOperationResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldService := backupService
	oldValidator := global.VALID
	global.VALID = validator.New()
	backupService = &backupSyncAPIService{status: dto.BackupSyncStatus{
		AccountName: "shared",
		Revision:    10,
		Status:      "sync_pending",
		Pending:     1,
		Total:       1,
		Targets: []dto.BackupSyncTargetStatus{
			{TargetKey: "node:2", NodeID: 2, NodeName: "remote", Status: "failed", DesiredRevision: 10, Attempts: 2, LastError: "retry diagnostic"},
		},
	}}
	t.Cleanup(func() {
		backupService = oldService
		global.VALID = oldValidator
	})

	router := gin.New()
	router.POST("/core/backups/sync/retry", ApiGroupApp.BaseApi.RetryBackupSync)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/core/backups/sync/retry",
		strings.NewReader(`{"name":"shared"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"applied":true`) || !strings.Contains(body, `"sync":{"accountName":"shared"`) {
		t.Fatalf("unexpected retry response: status=%d body=%s", recorder.Code, body)
	}
	for _, forbidden := range browserForbiddenBackupSyncFields() {
		if strings.Contains(body, forbidden) {
			t.Fatalf("retry response contains forbidden field/value %q: %s", forbidden, body)
		}
	}
}

func TestListBackupSyncStatusResponseIsSecretFree(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldService := backupService
	backupService = &backupSyncAPIService{status: dto.BackupSyncStatus{
		AccountName: "shared",
		Revision:    11,
		Status:      "partially_synced",
		Succeeded:   1,
		Pending:     1,
		Total:       2,
		Targets: []dto.BackupSyncTargetStatus{
			{
				TargetKey:       "node:2",
				NodeID:          2,
				NodeName:        "remote",
				Status:          "failed",
				AppliedRevision: 10,
				DesiredRevision: 11,
				LastError:       "internal synchronization diagnostic",
			},
		},
	}}
	t.Cleanup(func() { backupService = oldService })

	router := gin.New()
	router.GET("/core/backups/sync/status", ApiGroupApp.BaseApi.ListBackupSyncStatus)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/core/backups/sync/status", nil)
	router.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected list status=%d body=%s", recorder.Code, body)
	}
	for _, expected := range []string{`"accountName":"shared"`, `"status":"partially_synced"`, `"pending":1`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("list response does not contain %s: %s", expected, body)
		}
	}
	for _, forbidden := range browserForbiddenBackupSyncFields() {
		if strings.Contains(body, forbidden) {
			t.Fatalf("list response contains forbidden field/value %q: %s", forbidden, body)
		}
	}
}

func TestGetBackupSyncStatusResponseContainsOnlyAggregateState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldService := backupService
	backupService = &backupSyncAPIService{status: dto.BackupSyncStatus{
		AccountName: "shared",
		Revision:    12,
		Status:      "sync_pending",
		Pending:     1,
		Total:       1,
		Targets: []dto.BackupSyncTargetStatus{
			{
				TargetKey:       "node:2",
				NodeID:          2,
				NodeName:        "remote",
				Status:          "failed",
				DesiredRevision: 12,
				AppliedRevision: 11,
				Attempts:        3,
				NextRetryAt:     "2026-08-04T13:40:00Z",
				LastSuccessAt:   "2026-08-04T13:00:00Z",
				LastError:       "single status diagnostic",
			},
		},
	}}
	t.Cleanup(func() { backupService = oldService })

	router := gin.New()
	router.GET("/core/backups/sync/status/:name", ApiGroupApp.BaseApi.GetBackupSyncStatus)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/core/backups/sync/status/shared", nil)
	router.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected get status=%d body=%s", recorder.Code, body)
	}
	for _, expected := range []string{`"accountName":"shared"`, `"status":"sync_pending"`, `"pending":1`, `"total":1`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("get response does not contain %s: %s", expected, body)
		}
	}
	for _, forbidden := range append(browserForbiddenBackupSyncFields(), "single status diagnostic") {
		if strings.Contains(body, forbidden) {
			t.Fatalf("get response contains forbidden field/value %q: %s", forbidden, body)
		}
	}
}

func browserForbiddenBackupSyncFields() []string {
	return []string{
		"revision",
		"targets",
		"targetKey",
		"nodeId",
		"nodeName",
		"desiredRevision",
		"appliedRevision",
		"attempts",
		"nextRetryAt",
		"lastSuccessAt",
		"lastError",
		"clientSecret",
		"refreshToken",
		"snapshotDigest",
		"targetEpoch",
		"authority",
		"generation",
		"ciphertext",
		"synthetic-secret",
		"connection unavailable",
		"internal synchronization diagnostic",
	}
}
