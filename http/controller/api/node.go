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
	// region 可以是客户端自己配置的地域（优先），也可以是 api-server 为设备
	// 指定的地域；两者都没有时按纯分数排序，客户端仍会用实测延迟择优。
	region := c.Query("region")
	peerId := c.Query("peer_id")
	myRegion := ""
	if peerId != "" {
		myRegion = service.AllService.NodeService.PeerRegion(peerId)
	}
	if region == "" {
		region = myRegion
	}
	limit := 0
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	list := service.AllService.NodeService.Relays(region, limit)
	response.Success(c, gin.H{
		"relays":    list,
		"my_region": myRegion,
		"region":    region,
		"weights":   service.AllService.NodeService.Weights(),
	})
}

// ScoreConfig 读取调度权重（节点侧也可读取，便于本地日志排查）
// @Tags 集群节点
// @Summary 读取调度权重
// @Router /node/score-config [get]
func (n *Node) ScoreConfig(c *gin.Context) {
	response.Success(c, service.AllService.NodeService.Weights())
}

// Regions 返回可选地域列表（中国省级行政区划）
// @Tags 集群节点
// @Summary 地域列表
// @Router /node/regions [get]
func (n *Node) Regions(c *gin.Context) {
	response.Success(c, gin.H{"regions": service.ChinaProvinces})
}

// SelfRegion 客户端上报/查询自己的地域归属。
//
// 优先级约定：**客户端自己设置 > api-server 指定**。
// 客户端把本地配置的值通过 region 参数带上来（会被持久化到设备记录），
// 不传则只返回当前库里的值，供客户端在未配置时兜底使用。
// @Tags 集群节点
// @Summary 设备地域归属
// @Router /node/self-region [post]
func (n *Node) SelfRegion(c *gin.Context) {
	type Req struct {
		PeerId string `json:"peer_id"`
		Region string `json:"region"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if req.PeerId == "" {
		response.Fail(c, 101, "peer_id 不能为空")
		return
	}
	if req.Region != "" {
		// 客户端显式设置：落库，作为"客户端设置优先"的记录
		if err := service.AllService.NodeService.SetPeerRegion(req.PeerId, req.Region); err != nil {
			response.Fail(c, 101, err.Error())
			return
		}
	}
	stored := service.AllService.NodeService.PeerRegion(req.PeerId)
	response.Success(c, gin.H{"region": stored, "source": map[bool]string{true: "client", false: "server"}[req.Region != ""]})
}
// NodeConfig 节点拉取 api-server 下发的配置。
//
// 节点定期调用，按返回的 config 热应用能改的项（中继列表、限速等）；
// 无法热改的启动参数（key/端口/mask）由后台仅做展示。
// @Tags 集群节点
// @Summary 拉取节点配置
// @Router /node/config [get]
func (n *Node) NodeConfig(c *gin.Context) {
	nodeId := c.Query("node_id")
	if nodeId == "" {
		response.Fail(c, 101, "node_id 不能为空")
		return
	}
	cfg, version, err := service.AllService.NodeService.GetConfig(nodeId)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"config": cfg, "version": version})
}