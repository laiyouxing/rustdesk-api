package admin

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
)

type ServerStatus struct {
}

// Status 检查 hbbs/hbbr 端口连通性，仅管理员可访问
// @Tags 系统
// @Summary 服务器状态
// @Description 通过 TCP 探测 hbbs(rendezvous) 与 hbbr(relay) 服务端口，返回连通性与延迟
// @Produce  json
// @Success 200 {object} response.Response
// @Router /server_status [get]
// @Security token
func (ct *ServerStatus) Status(c *gin.Context) {
	hbbs := probe(global.Config.Rustdesk.IdServer, global.Config.Admin.IdServerPort)
	hbbr := probe(global.Config.Rustdesk.RelayServer, global.Config.Admin.RelayServerPort)
	response.Success(c, gin.H{
		"hbbs": hbbs,
		"hbbr": hbbr,
	})
}

// probe 通过 TCP 探测指定地址，返回状态/延迟/错误信息
func probe(host string, defaultPort int) gin.H {
	if host == "" {
		return gin.H{
			"host":       "",
			"status":     "not_configured",
			"latency_ms": 0,
			"error":      "",
		}
	}
	addr := host
	if !strings.Contains(host, ":") {
		addr = net.JoinHostPort(host, strconv.Itoa(defaultPort))
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return gin.H{
			"host":       addr,
			"status":     "down",
			"latency_ms": 0,
			"error":      err.Error(),
		}
	}
	defer conn.Close()
	return gin.H{
		"host":       addr,
		"status":     "up",
		"latency_ms": time.Since(start).Milliseconds(),
		"error":      "",
	}
}
