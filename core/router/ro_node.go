package router

import (
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
	}

	// Node enrollment endpoint: called by a joining node that has no session.
	// It carries no session cookie, so it is authenticated by the single-use
	// HMAC enrollment token in the body (CSRF is skipped for cookieless calls).
	enrollRouter := Router.Group("nodes")
	{
		enrollRouter.POST("/enroll", baseApi.EnrollNode)
	}
}
