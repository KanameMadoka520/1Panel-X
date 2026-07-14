package v2

import (
	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/gin-gonic/gin"
)

// @Tags Website
// @Summary Load website access stat
// @Accept json
// @Param id path integer true "website id"
// @Param request body request.WebsiteMonitorSearch true "request"
// @Success 200 {array} model.WebsiteAccessStat
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/monitor/stat [post]
func (b *BaseApi) LoadWebsiteAccessStat(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WebsiteMonitorSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := websiteStatService.LoadStat(id, req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

// @Tags Website
// @Summary Load website access rank
// @Accept json
// @Param id path integer true "website id"
// @Param request body request.WebsiteMonitorSearch true "request"
// @Success 200 {array} model.WebsiteAccessRank
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/monitor/rank [post]
func (b *BaseApi) LoadWebsiteAccessRank(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WebsiteMonitorSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := websiteStatService.LoadRank(id, req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}
