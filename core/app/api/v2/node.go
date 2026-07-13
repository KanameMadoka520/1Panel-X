package v2

import (
	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags Node
// @Summary List nodes
// @Accept json
// @Param request body dto.NodeSearch true "request"
// @Success 200 {array} dto.NodeInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/nodes/list [post]
func (b *BaseApi) ListNodes(c *gin.Context) {
	var req dto.NodeSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	items, err := nodeService.List(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, items)
}

// @Tags Node
// @Summary List simple nodes
// @Success 200 {array} dto.SimpleNodeInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/nodes/simple/all [get]
func (b *BaseApi) ListSimpleNodes(c *gin.Context) {
	items, err := nodeService.SimpleAll()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, items)
}

// @Tags Node
// @Summary Register a node and mint its enrollment token
// @Accept json
// @Param request body dto.NodeCreate true "request"
// @Success 200 {object} dto.NodeEnrollToken
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/nodes [post]
// @x-panel-log {"bodyKeys":["name","addr"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"注册节点 [name][addr]","formatEN":"register node [name][addr]"}
func (b *BaseApi) CreateNode(c *gin.Context) {
	var req dto.NodeCreate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	token, err := nodeService.Create(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, token)
}

// @Tags Node
// @Summary Delete a node
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/nodes/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"nodes","output_column":"name","output_value":"name"}],"formatZH":"删除节点 [name]","formatEN":"delete node [name]"}
func (b *BaseApi) DeleteNode(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := nodeService.Delete(req.ID); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Node
// @Summary Revoke a node (keeps the registry row for audit)
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/nodes/revoke [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"nodes","output_column":"name","output_value":"name"}],"formatZH":"撤销节点 [name]","formatEN":"revoke node [name]"}
func (b *BaseApi) RevokeNode(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := nodeService.Revoke(req.ID); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Node
// @Summary Enroll a node (token-gated, called by a joining node)
// @Accept json
// @Param request body dto.NodeEnrollRequest true "request"
// @Success 200 {object} dto.NodeEnrollResponse
// @Router /core/nodes/enroll [post]
func (b *BaseApi) EnrollNode(c *gin.Context) {
	var req dto.NodeEnrollRequest
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	resp, err := nodeService.Enroll(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, resp)
}
