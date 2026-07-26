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

func (b *BaseApi) GetWafLists(c *gin.Context) {
	res, err := wafListService.List()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) SaveWafListEntry(c *gin.Context) {
	var req request.WafListEntrySave
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafListService.SaveEntry(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) DeleteWafListEntries(c *gin.Context) {
	var req request.WafListDelete
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafListService.DeleteEntries(req.IDs)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) SaveWafIPGroup(c *gin.Context) {
	var req request.WafIPGroupSave
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafListService.SaveIPGroup(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) DeleteWafIPGroups(c *gin.Context) {
	var req request.WafListDelete
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafListService.DeleteIPGroups(req.IDs)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) GetWafBans(c *gin.Context) {
	res, err := wafBanService.List()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

func (b *BaseApi) ReleaseWafBan(c *gin.Context) {
	var req request.WafBanRelease
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafBanService.Release(req.IP)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website
// @Summary List a website's WAF upload restriction rules
// @Param id path integer true "website id"
// @Success 200 {object} response.WafUploadRules
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/uploads [get]
func (b *BaseApi) GetWafUploadRules(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	res, err := wafUploadRuleService.List(id)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website
// @Summary Create or update a WAF upload restriction rule
// @Accept json
// @Param id path integer true "website id"
// @Param request body request.WafUploadRuleSave true "request"
// @Success 200 {object} response.WafUploadRules
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/uploads [post]
func (b *BaseApi) SaveWafUploadRule(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WafUploadRuleSave
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafUploadRuleService.Save(id, req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website
// @Summary Delete WAF upload restriction rules
// @Accept json
// @Param id path integer true "website id"
// @Param request body request.WafListDelete true "request"
// @Success 200 {object} response.WafUploadRules
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/uploads/del [post]
func (b *BaseApi) DeleteWafUploadRules(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WafListDelete
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafUploadRuleService.Delete(id, req.IDs)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website
// @Summary Switch a website's WAF upload restriction on or off
// @Accept json
// @Param id path integer true "website id"
// @Param request body request.WafUploadToggle true "request"
// @Success 200 {object} response.WafUploadRules
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/uploads/toggle [post]
func (b *BaseApi) ToggleWafUploadRules(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WafUploadToggle
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafUploadRuleService.SetEnabled(id, req.Enabled)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website
// @Summary List WAF custom rules
// @Success 200 {array} response.WafCustomRule
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/rules [get]
func (b *BaseApi) GetWafCustomRules(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	res, err := wafCustomRuleService.List(id)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website
// @Summary Create or update a WAF custom rule
// @Accept json
// @Param request body request.WafCustomRuleSave true "request"
// @Success 200 {array} response.WafCustomRule
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/rules [post]
func (b *BaseApi) SaveWafCustomRule(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WafCustomRuleSave
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafCustomRuleService.Save(id, req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website
// @Summary Delete WAF custom rules
// @Accept json
// @Param request body request.WafListDelete true "request"
// @Success 200 {array} response.WafCustomRule
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/rules/del [post]
func (b *BaseApi) DeleteWafCustomRules(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WafListDelete
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafCustomRuleService.Delete(id, req.IDs)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website
// @Summary Reorder WAF custom rules
// @Accept json
// @Param request body request.WafCustomRuleReorder true "request"
// @Success 200 {array} response.WafCustomRule
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/{id}/waf/rules/order [post]
func (b *BaseApi) ReorderWafCustomRules(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	var req request.WafCustomRuleReorder
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := wafCustomRuleService.Reorder(id, req.IDs)
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
