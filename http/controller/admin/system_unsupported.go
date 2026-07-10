//go:build !linux

package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
)

// ServiceRestart 非 Linux 平台不支持在线重启，提示手动重启
func (co *Config) ServiceRestart(c *gin.Context) {
	response.Fail(c, 400, "当前平台不支持在线重启服务，请手动重启进程")
}
