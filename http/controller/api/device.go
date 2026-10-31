package api

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Device struct {
}

// ClaimId 为新装设备领取远程 ID（需登录）
// @Tags Device
// @Summary 领取远程 ID
// @Description 新装客户端登录后调用，上报本机设备码(uuid)，服务端分配并返回 9 位数字 ID。
// @Description ID 绑定设备本身，换账号不变；已有 ID 的设备幂等返回原值。
// @Accept  json
// @Produce  json
// @Param body body object true "{uuid, hostname}"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.ErrorResponse
// @Router /api/device/claim-id [post]
func (d *Device) ClaimId(c *gin.Context) {
	form := &struct {
		Uuid     string `json:"uuid"`
		Hostname string `json:"hostname"`
	}{}
	if err := c.ShouldBindJSON(form); err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if form.Uuid == "" {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+"uuid")
		return
	}
	u := service.AllService.UserService.CurUser(c)
	if u == nil || u.Id == 0 {
		// 必须先登录才允许领取：这是本功能的核心约束
		response.Error(c, "请先登录后再获取远程 ID")
		return
	}
	id, err := service.AllService.PeerService.ClaimId(form.Uuid, u.Id, form.Hostname)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id})
}
