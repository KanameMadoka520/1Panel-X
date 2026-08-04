package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/gin-gonic/gin"
)

func TestValidProxyIDFailsClosed(t *testing.T) {
	oldPath := nodeProxyIDPath
	t.Cleanup(func() { nodeProxyIDPath = oldPath })

	t.Run("missing file", func(t *testing.T) {
		nodeProxyIDPath = filepath.Join(t.TempDir(), "missing")
		if validProxyID("proxy-id") {
			t.Fatal("missing Proxy-Id file was accepted")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		nodeProxyIDPath = writeProxyIDFile(t, "")
		if validProxyID("proxy-id") {
			t.Fatal("empty stored Proxy-Id was accepted")
		}
	})

	t.Run("empty request header", func(t *testing.T) {
		nodeProxyIDPath = writeProxyIDFile(t, "proxy-id")
		if validProxyID("") {
			t.Fatal("empty request Proxy-Id was accepted")
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		nodeProxyIDPath = writeProxyIDFile(t, "proxy-id")
		if validProxyID("other-id") {
			t.Fatal("mismatched Proxy-Id was accepted")
		}
	})

	t.Run("match", func(t *testing.T) {
		nodeProxyIDPath = writeProxyIDFile(t, "proxy-id\n")
		if !validProxyID(" proxy-id ") {
			t.Fatal("matching Proxy-Id was rejected")
		}
	})
}

func TestCertificateRequiresProxyIDForUpgradeRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldMaster := global.IsMaster
	oldPath := nodeProxyIDPath
	oldValidate := validateNodeCertificate
	global.IsMaster = false
	nodeProxyIDPath = writeProxyIDFile(t, "proxy-id")
	validateNodeCertificate = func(*gin.Context) bool { return true }
	t.Cleanup(func() {
		global.IsMaster = oldMaster
		nodeProxyIDPath = oldPath
		validateNodeCertificate = oldValidate
	})

	served := false
	router := gin.New()
	router.Use(Certificate())
	router.GET("/upgrade", func(c *gin.Context) {
		served = true
		c.Status(http.StatusOK)
	})

	missingRecorder := httptest.NewRecorder()
	missingRequest := httptest.NewRequest(http.MethodGet, "/upgrade", nil)
	missingRequest.Header.Set("Connection", "Upgrade")
	router.ServeHTTP(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusForbidden || served {
		t.Fatalf("upgrade without Proxy-Id was not rejected: status=%d served=%v", missingRecorder.Code, served)
	}

	validRecorder := httptest.NewRecorder()
	validRequest := httptest.NewRequest(http.MethodGet, "/upgrade", nil)
	validRequest.Header.Set("Connection", "Upgrade")
	validRequest.Header.Set("Proxy-Id", "proxy-id")
	router.ServeHTTP(validRecorder, validRequest)
	if validRecorder.Code != http.StatusOK || !served {
		t.Fatalf("upgrade with matching Proxy-Id was rejected: status=%d served=%v", validRecorder.Code, served)
	}
}

func TestCertificateRejectsInvalidClientCertificate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldMaster := global.IsMaster
	oldValidate := validateNodeCertificate
	global.IsMaster = false
	validateNodeCertificate = func(*gin.Context) bool { return false }
	t.Cleanup(func() {
		global.IsMaster = oldMaster
		validateNodeCertificate = oldValidate
	})

	served := false
	router := gin.New()
	router.Use(Certificate())
	router.GET("/private", func(c *gin.Context) { served = true })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/private", nil))
	if recorder.Code != http.StatusForbidden || served {
		t.Fatalf("invalid client certificate was not rejected: status=%d served=%v", recorder.Code, served)
	}
}

func writeProxyIDFile(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".nodeProxyID")
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
