package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/gin-gonic/gin"
)

type agentOAuthAPIBackupService struct {
	service.IBackupService
	info dto.OAuthCredentialInfo
}

func (f *agentOAuthAPIBackupService) GetOAuthCredential(uint) (dto.OAuthCredentialInfo, error) {
	return f.info, nil
}

func TestGetBackupOAuthCredentialResponseIsSecretFree(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldService := backupService
	backupService = &agentOAuthAPIBackupService{info: dto.OAuthCredentialInfo{
		Provider:                "google",
		Configured:              true,
		Authorized:              false,
		ClientIDDisplay:         "admi...t-id",
		RedirectURI:             "http://localhost/login/authorized",
		Status:                  "reauthorization_required",
		RequiresReauthorization: true,
		UpdatedAt:               "2026-08-03T00:00:00Z",
	}}
	t.Cleanup(func() { backupService = oldService })

	router := gin.New()
	router.GET("/backups/oauth/credential/:id", ApiGroupApp.BaseApi.GetBackupOAuthCredential)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/backups/oauth/credential/7", nil)
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
