package v2

import (
	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// GetPublicEnhancementSetting returns visual settings that are safe to expose before login.
func (b *BaseApi) GetPublicEnhancementSetting(c *gin.Context) {
	setting, err := enhancementService.GetPublicSettingInfo()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, setting)
}

// GetEnhancementSetting returns the complete open enhancement settings for an authenticated user.
func (b *BaseApi) GetEnhancementSetting(c *gin.Context) {
	setting, err := enhancementService.GetSettingInfo()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, setting)
}

// UpdateEnhancementSetting updates an independently implemented open enhancement setting.
func (b *BaseApi) UpdateEnhancementSetting(c *gin.Context) {
	var req dto.EnhancementSettingUpdate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := enhancementService.UpdateSetting(req.Key, req.Value); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}
