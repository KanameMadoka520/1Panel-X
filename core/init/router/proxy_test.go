package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProxyRejectsBrowserAccessToPublicBackupSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Proxy())
	router.Any("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, internalPublicBackupSyncPath, nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("internal backup sync proxy status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
