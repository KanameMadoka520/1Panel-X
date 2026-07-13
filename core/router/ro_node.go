package router

import (
	"net/http"
	"sync"
	"time"

	v2 "github.com/1Panel-dev/1Panel/core/app/api/v2"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/gin-gonic/gin"
)

type NodeRouter struct {
}

func (a *NodeRouter) InitRouter(Router *gin.RouterGroup) {
	baseApi := v2.ApiGroupApp.BaseApi

	// Admin-facing node management (session-authenticated).
	nodeRouter := Router.Group("nodes").
		Use(middleware.SessionAuth()).
		Use(middleware.PasswordExpired())
	{
		nodeRouter.POST("/list", baseApi.ListNodes)
		nodeRouter.GET("/simple/all", baseApi.ListSimpleNodes)
		nodeRouter.POST("", baseApi.CreateNode)
		nodeRouter.POST("/del", baseApi.DeleteNode)
		nodeRouter.POST("/revoke", baseApi.RevokeNode)
	}

	// Node enrollment endpoint: called by a joining node that has no session.
	// It carries no session cookie, so it is authenticated by the single-use
	// HMAC enrollment token in the body (CSRF is skipped for cookieless calls).
	// A per-IP rate limit bounds unauthenticated request volume (N13 defense in
	// depth; the token itself is the primary control).
	enrollRouter := Router.Group("nodes")
	enrollRouter.Use(enrollRateLimit())
	{
		enrollRouter.POST("/enroll", baseApi.EnrollNode)
	}
}

// enrollLimiter is a tiny per-IP fixed-window limiter for the sessionless
// enrollment endpoint. Kept dependency-free (no golang.org/x/time in core).
var enrollLimiter = &fixedWindowLimiter{max: 10, window: time.Minute, seen: map[string]*windowCount{}}

func enrollRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enrollLimiter.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "too many enrollment attempts, please retry later",
			})
			return
		}
		c.Next()
	}
}

type windowCount struct {
	start time.Time
	count int
}

type fixedWindowLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	seen   map[string]*windowCount
}

func (l *fixedWindowLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	// opportunistic prune so the map cannot grow unbounded with distinct IPs
	if len(l.seen) > 1024 {
		for k, v := range l.seen {
			if now.Sub(v.start) > l.window {
				delete(l.seen, k)
			}
		}
	}
	wc, ok := l.seen[ip]
	if !ok || now.Sub(wc.start) > l.window {
		l.seen[ip] = &windowCount{start: now, count: 1}
		return true
	}
	if wc.count >= l.max {
		return false
	}
	wc.count++
	return true
}
