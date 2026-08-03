package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/service"
	"github.com/gin-gonic/gin"
)

type coreOAuthAPIBackupService struct {
	service.IBackupService
	info dto.OAuthCredentialInfo
}

func (f *coreOAuthAPIBackupService) GetOAuthCredential(string) (dto.OAuthCredentialInfo, error) {
	return f.info, nil
}

func TestGetBackupOAuthCredentialResponseIsSecretFree(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldService := backupService
	backupService = &coreOAuthAPIBackupService{info: dto.OAuthCredentialInfo{
		Provider:                "microsoft",
		Configured:              true,
		Authorized:              true,
		ClientIDDisplay:         "admi...t-id",
		RedirectURI:             "http://localhost/login/authorized",
		Status:                  "configured",
		RequiresReauthorization: false,
		UpdatedAt:               "2026-08-03T00:00:00Z",
	}}
	t.Cleanup(func() { backupService = oldService })

	router := gin.New()
	router.GET("/core/backups/oauth/credential/:name", ApiGroupApp.BaseApi.GetBackupOAuthCredential)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/core/backups/oauth/credential/shared", nil)
	router.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"clientIdDisplay":"admi...t-id"`) {
		t.Fatalf("unexpected OAuth credential response: status=%d body=%s", recorder.Code, body)
	}
	for _, forbidden := range []string{
		"synthetic-client-secret", "synthetic-refresh-token", "clientSecret", "refreshToken", "OneDriveSc", "GoogleSc",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("OAuth credential API response contains %q", forbidden)
		}
	}
}

func TestLoadBackupClientInfoCompatibilityResponseIsSecretFree(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldService := backupService
	backupService = service.NewIBackupService()
	t.Cleanup(func() { backupService = oldService })

	router := gin.New()
	router.GET("/core/backups/client/:clientType", ApiGroupApp.BaseApi.LoadBackupClientInfo)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/core/backups/client/OneDrive", nil)
	router.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"configured":false`) {
		t.Fatalf("unexpected OAuth client compatibility response: status=%d body=%s", recorder.Code, body)
	}
	for _, forbidden := range []string{
		"clientSecret", "refreshToken", "client_secret", "refresh_token", "OneDriveSc", "GoogleSc",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("OAuth client compatibility response contains %q", forbidden)
		}
	}
}
