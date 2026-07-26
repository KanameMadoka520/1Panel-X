package router

import (
	v2 "github.com/1Panel-dev/1Panel/agent/app/api/v2"
	"github.com/gin-gonic/gin"
)

type WebsiteRouter struct {
}

func (a *WebsiteRouter) InitRouter(Router *gin.RouterGroup) {
	websiteRouter := Router.Group("websites")

	baseApi := v2.ApiGroupApp.BaseApi
	{
		websiteRouter.POST("/search", baseApi.PageWebsite)
		websiteRouter.GET("/list", baseApi.GetWebsites)
		websiteRouter.POST("", baseApi.CreateWebsite)
		websiteRouter.POST("/operate", baseApi.OpWebsite)
		websiteRouter.POST("/log/search", baseApi.GetWebsiteLog)
		websiteRouter.POST("/log/operate", baseApi.OpWebsiteLog)
		websiteRouter.POST("/check", baseApi.CreateWebsiteCheck)
		websiteRouter.POST("/options", baseApi.GetWebsiteOptions)
		websiteRouter.POST("/update", baseApi.UpdateWebsite)
		websiteRouter.GET("/:id", baseApi.GetWebsite)
		websiteRouter.POST("/del", baseApi.DeleteWebsite)
		websiteRouter.POST("/default/server", baseApi.ChangeDefaultServer)
		websiteRouter.POST("/group/change", baseApi.ChangeWebsiteGroup)

		websiteRouter.POST("/batch/operate", baseApi.BatchOpWebsites)
		websiteRouter.POST("/batch/group", baseApi.BatchSetWebsiteGroup)
		websiteRouter.POST("/batch/ssl", baseApi.BatchSetHttps)

		websiteRouter.GET("/domains/:websiteId", baseApi.GetWebDomains)
		websiteRouter.POST("/domains/del", baseApi.DeleteWebDomain)
		websiteRouter.POST("/domains", baseApi.CreateWebDomain)
		websiteRouter.POST("/domains/update", baseApi.UpdateWebDomain)

		websiteRouter.GET("/:id/config/:type", baseApi.GetWebsiteNginx)
		websiteRouter.POST("/config", baseApi.GetNginxConfig)
		websiteRouter.POST("/config/update", baseApi.UpdateNginxConfig)
		websiteRouter.POST("/nginx/update", baseApi.UpdateWebsiteNginxConfig)

		websiteRouter.GET("/:id/https", baseApi.GetHTTPSConfig)
		websiteRouter.POST("/:id/https", baseApi.UpdateHTTPSConfig)

		websiteRouter.POST("/rewrite", baseApi.GetRewriteConfig)
		websiteRouter.POST("/rewrite/update", baseApi.UpdateRewriteConfig)
		websiteRouter.POST("/rewrite/custom", baseApi.OperateCustomRewrite)
		websiteRouter.GET("/rewrite/custom", baseApi.ListCustomRewrite)

		websiteRouter.POST("/dir/update", baseApi.UpdateSiteDir)
		websiteRouter.POST("/dir/permission", baseApi.UpdateSiteDirPermission)
		websiteRouter.POST("/dir", baseApi.GetDirConfig)

		websiteRouter.POST("/proxies", baseApi.GetProxyConfig)
		websiteRouter.POST("/proxies/update", baseApi.UpdateProxyConfig)
		websiteRouter.POST("/proxies/delete", baseApi.DeleteProxyConfig)
		websiteRouter.POST("/proxies/status", baseApi.UpdateProxyConfigStatus)
		websiteRouter.POST("/proxies/file", baseApi.UpdateProxyConfigFile)
		websiteRouter.POST("/proxy/config", baseApi.UpdateProxyCache)
		websiteRouter.GET("/proxy/config/:id", baseApi.GetProxyCache)
		websiteRouter.POST("/proxy/clear", baseApi.ClearProxyCache)

		websiteRouter.POST("/auths", baseApi.GetAuthConfig)
		websiteRouter.POST("/auths/update", baseApi.UpdateAuthConfig)
		websiteRouter.POST("/auths/path", baseApi.GetPathAuthConfig)
		websiteRouter.POST("/auths/path/update", baseApi.UpdatePathAuthConfig)

		websiteRouter.GET("/cors/:id", baseApi.GetCORSConfig)
		websiteRouter.POST("/cors/update", baseApi.UpdateCORSConfig)

		websiteRouter.POST("/leech", baseApi.GetAntiLeech)
		websiteRouter.POST("/leech/update", baseApi.UpdateAntiLeech)

		websiteRouter.POST("/redirect/update", baseApi.UpdateRedirectConfig)
		websiteRouter.POST("/redirect", baseApi.GetRedirectConfig)
		websiteRouter.POST("/redirect/file", baseApi.UpdateRedirectConfigFile)

		websiteRouter.GET("/default/html/:type", baseApi.GetDefaultHtml)
		websiteRouter.POST("/default/html/update", baseApi.UpdateDefaultHtml)

		websiteRouter.GET("/:id/lbs", baseApi.GetLoadBalances)
		websiteRouter.POST("/lbs/create", baseApi.CreateLoadBalance)
		websiteRouter.POST("/lbs/del", baseApi.DeleteLoadBalance)
		websiteRouter.POST("/lbs/update", baseApi.UpdateLoadBalance)
		websiteRouter.POST("/lbs/file", baseApi.UpdateLoadBalanceFile)

		websiteRouter.POST("/php/version", baseApi.ChangePHPVersion)

		websiteRouter.POST("/realip/config", baseApi.SetRealIPConfig)
		websiteRouter.GET("/realip/config/:id", baseApi.GetRealIPConfig)

		websiteRouter.POST("/:id/monitor/stat", baseApi.LoadWebsiteAccessStat)
		websiteRouter.POST("/:id/monitor/rank", baseApi.LoadWebsiteAccessRank)
		websiteRouter.POST("/:id/waf/events", baseApi.LoadWafEvents)
		websiteRouter.GET("/:id/waf/status", baseApi.GetWafStatus)
		websiteRouter.POST("/:id/waf/config", baseApi.UpdateWafSite)
		websiteRouter.GET("/waf/global", baseApi.GetWafGlobal)
		websiteRouter.POST("/waf/global", baseApi.UpdateWafGlobal)
		websiteRouter.GET("/waf/lists", baseApi.GetWafLists)
		websiteRouter.POST("/waf/lists/entry", baseApi.SaveWafListEntry)
		websiteRouter.POST("/waf/lists/entry/del", baseApi.DeleteWafListEntries)
		websiteRouter.POST("/waf/lists/group", baseApi.SaveWafIPGroup)
		websiteRouter.POST("/waf/lists/group/del", baseApi.DeleteWafIPGroups)
			websiteRouter.GET("/:id/waf/uploads", baseApi.GetWafUploadRules)
			websiteRouter.POST("/:id/waf/uploads", baseApi.SaveWafUploadRule)
			websiteRouter.POST("/:id/waf/uploads/del", baseApi.DeleteWafUploadRules)
			websiteRouter.POST("/:id/waf/uploads/toggle", baseApi.ToggleWafUploadRules)
			websiteRouter.GET("/waf/rules", baseApi.GetWafCustomRules)
			websiteRouter.POST("/waf/rules", baseApi.SaveWafCustomRule)
			websiteRouter.POST("/waf/rules/del", baseApi.DeleteWafCustomRules)
			websiteRouter.POST("/waf/rules/order", baseApi.ReorderWafCustomRules)
			websiteRouter.GET("/waf/bans", baseApi.GetWafBans)
		websiteRouter.POST("/waf/bans/release", baseApi.ReleaseWafBan)

		websiteRouter.GET("/resource/:id", baseApi.GetWebsiteResource)
		websiteRouter.GET("/databases", baseApi.GetWebsiteDatabase)
		websiteRouter.POST("/databases", baseApi.ChangeWebsiteDatabase)

		websiteRouter.POST("/crosssite", baseApi.OperateCrossSiteAccess)

		websiteRouter.POST("/exec/composer", baseApi.ExecComposer)
		websiteRouter.POST("/stream/update", baseApi.UpdateStreamConfig)
	}
}
