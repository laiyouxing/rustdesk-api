package api

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Version struct {
}

// LatestVersion 获取最新版本
// @Tags 版本检测
// @Summary 获取最新版本信息
// @Description 客户端调用该接口检查是否有新版本
// @Accept  json
// @Produce  json
// @Param platform query string false "平台: windows/macos/linux/ubuntu"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/version/latest [get]
func (v *Version) LatestVersion(c *gin.Context) {
	platform := c.DefaultQuery("platform", "")
	ver := service.AllService.AppReleaseService.Latest(platform)
	if ver == nil || ver.Id == 0 {
		response.Success(c, gin.H{
			"version":  "",
			"url":      "",
			"platform": platform,
			"note":     "",
		})
		return
	}
	response.Success(c, gin.H{
		"version":  ver.Version,
		"url":      ver.Url,
		"platform": ver.Platform,
		"note":     ver.Note,
	})
}
