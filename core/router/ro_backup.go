package router

import (
	v2 "github.com/1Panel-dev/1Panel/core/app/api/v2"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/gin-gonic/gin"
)

type BackupRouter struct{}

func (s *BackupRouter) InitRouter(Router *gin.RouterGroup) {
	backupRouter := Router.Group("backups").
		Use(middleware.SessionAuth()).
		Use(middleware.PasswordExpired())
	baseApi := v2.ApiGroupApp.BaseApi
	{
		backupRouter.GET("/client/:clientType", baseApi.LoadBackupClientInfo)
		backupRouter.GET("/oauth/credential/:name", baseApi.GetBackupOAuthCredential)
		backupRouter.POST("/oauth/begin", baseApi.BeginBackupOAuth)
		backupRouter.POST("/oauth/complete", baseApi.CompleteBackupOAuth)
		backupRouter.POST("/oauth/credential/clear", baseApi.ClearBackupOAuthCredential)
		backupRouter.POST("/refresh/token", baseApi.RefreshToken)
		backupRouter.POST("", baseApi.CreateBackup)
		backupRouter.POST("/del", baseApi.DeleteBackup)
		backupRouter.POST("/update", baseApi.UpdateBackup)
	}
}
