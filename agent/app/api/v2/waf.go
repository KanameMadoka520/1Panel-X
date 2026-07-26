package v2

import (
	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/gin-gonic/gin"
)

// @Tags Website
// @Summary Load website WAF attack events
// @Accept json
// @Param id path integer true "website id"
// @Param request body request.WafEventSearch true "request"
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/events [post]
func (b *BaseApi) LoadWafEvents(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WafEventSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafEventService.LoadEvents(id, req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) GetWafStatus(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	res, err := wafControlService.GetStatus(id)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) UpdateWafSite(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WafSiteUpdate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafControlService.Update(id, req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) GetWafGlobal(c *gin.Context) {
	res, err := wafControlService.GetGlobal()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) UpdateWafGlobal(c *gin.Context) {
	var req request.WafGlobalUpdate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafControlService.UpdateGlobal(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}
