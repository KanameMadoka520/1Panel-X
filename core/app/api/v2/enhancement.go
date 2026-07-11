package v2

import (
	"io"
	"net/http"

	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// enhancementAssetBodyLimit caps the whole multipart upload body before it is
// parsed, bounding memory (T4). It is the largest per-asset limit (2 MiB) plus
// slack for multipart framing; the service re-checks the tighter per-asset cap
// on the decoded file bytes authoritatively.
const enhancementAssetBodyLimit = (2 << 20) + (64 << 10)

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

// UploadEnhancementAsset validates and stores a branding image (logo, favicon,
// login image, ...). The asset key is a fixed server-side enum selecting a
// hardcoded destination; the client's own filename is never used.
func (b *BaseApi) UploadEnhancementAsset(c *gin.Context) {
	// Bound the request body before parsing the multipart form (T4).
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, enhancementAssetBodyLimit)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	assetKey := c.PostForm("key")
	file, err := fileHeader.Open()
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	if err := enhancementService.SaveAsset(assetKey, data); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// ResetEnhancementAsset removes a custom branding image and restores the
// built-in default.
func (b *BaseApi) ResetEnhancementAsset(c *gin.Context) {
	var req dto.EnhancementAssetReset
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := enhancementService.ResetAsset(req.Key); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}
