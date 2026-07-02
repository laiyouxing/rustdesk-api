package api

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type ClientDownload struct{}

// List 获取启用的客户端下载列表（公开）
func (v *ClientDownload) List(c *gin.Context) {
	list := service.AllService.ClientDownloadService.ActiveList()
	response.Success(c, gin.H{
		"list": list,
	})
}
