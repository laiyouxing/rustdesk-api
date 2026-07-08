package my

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
)

type Audit struct {
}

// List 普通用户的远程连接记录列表
func (a *Audit) List(c *gin.Context) {
	query := &admin.AuditQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	res := service.AllService.AuditService.AuditConnList(query.Page, query.PageSize, func(tx *gorm.DB) {
		// 普通用户只能看到自己设备发起的连接（from_peer 是自己的设备ID）
		var peerIds []string
		service.DB.Model(&model.Peer{}).Where("user_id = ?", u.Id).Pluck("id", &peerIds)
		if len(peerIds) > 0 {
			tx.Where("from_peer in ?", peerIds)
		} else {
			tx.Where("1 = 0")
		}
		tx.Where("action = 'new'")
		tx.Order("id desc")
	})
	response.Success(c, res)
}
