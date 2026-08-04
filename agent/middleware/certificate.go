package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack"
	"github.com/gin-gonic/gin"
)

var (
	nodeProxyIDPath         = "/etc/1panel/.nodeProxyID"
	validateNodeCertificate = func(c *gin.Context) bool { return xpack.MultiNodeProvider.ValidateCertificate(c) }
)

func Certificate() gin.HandlerFunc {
	return func(c *gin.Context) {
		if global.IsMaster {
			c.Next()
			return
		}
		if !validateNodeCertificate(c) {
			CloseDirectly(c)
			return
		}
		if !validProxyID(c.Request.Header.Get("Proxy-Id")) {
			CloseDirectly(c)
			return
		}
		c.Next()
	}
}

func validProxyID(requestProxyID string) bool {
	requestProxyID = strings.TrimSpace(requestProxyID)
	if requestProxyID == "" {
		return false
	}
	stored, err := os.ReadFile(nodeProxyIDPath)
	if err != nil {
		return false
	}
	expectedProxyID := strings.TrimSpace(string(stored))
	if expectedProxyID == "" || len(expectedProxyID) != len(requestProxyID) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expectedProxyID), []byte(requestProxyID)) == 1
}

func CloseDirectly(c *gin.Context) {
	c.Abort()
	c.Status(http.StatusForbidden)
	defer func() {
		_ = recover()
	}()
	hijacker, ok := http.Hijacker(c.Writer), true
	if unwrapper, canUnwrap := c.Writer.(interface{ Unwrap() http.ResponseWriter }); canUnwrap {
		hijacker, ok = unwrapper.Unwrap().(http.Hijacker)
	}
	if !ok {
		c.Writer.WriteHeaderNow()
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		c.Status(http.StatusForbidden)
		return
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetLinger(0)
	}
	_ = conn.Close()
}
