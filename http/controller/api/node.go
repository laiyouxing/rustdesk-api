package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// Node 集群节点的自助接口（hbbs / hbbr 调用）。
//
// 鉴权：复用 ProcessMonitorAuth（要求携带与服务器一致的 rustdesk key，
// 或已登录的 JWT）。节点部署在公网，所以按"公网 + 密钥"这一约定。
type Node struct{}

// Register 节点注册（幂等）
// @Tags 集群节点
// @Summary 节点注册
// @Router /node/register [post]
func (n *Node) Register(c *gin.Context) {
	f := &service.NodeRegisterForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	node, err := service.AllService.NodeService.Register(f)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"id": node.Id, "node_id": node.NodeId})
}

// Heartbeat 节点心跳
// @Tags 集群节点
// @Summary 节点心跳（上报 peer 数 / 会话数 / 带宽 / CPU / 内存）
// @Router /node/heartbeat [post]
func (n *Node) Heartbeat(c *gin.Context) {
	f := &service.NodeHeartbeatForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if err := service.AllService.NodeService.Heartbeat(f); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// Relays 拉取按调度分数排序的中继列表（hbbs 用来替代静态 -r）
// @Tags 集群节点
// @Summary 拉取排好序的中继列表
// @Router /node/relays [get]
func (n *Node) Relays(c *gin.Context) {
	region := c.Query("region")
	limit := 0
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	list := service.AllService.NodeService.Relays(region, limit)
	response.Success(c, gin.H{"relays": list, "weights": service.AllService.NodeService.Weights()})
}

// ScoreConfig 读取调度权重（节点侧也可读取，便于本地日志排查）
// @Tags 集群节点
// @Summary 读取调度权重
// @Router /node/score-config [get]
func (n *Node) ScoreConfig(c *gin.Context) {
	response.Success(c, service.AllService.NodeService.Weights())
}
