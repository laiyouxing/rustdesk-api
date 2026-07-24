package api

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Announcement struct {
}

// List 客户端获取公告列表
// @Tags 公告
// @Summary 客户端获取公告列表
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Router /api/announcements [get]
func (a *Announcement) List(c *gin.Context) {
	announcements := service.AllService.AnnouncementService.ListActiveForClient()
	response.Success(c, gin.H{
		"announcements": announcements,
	})
}
