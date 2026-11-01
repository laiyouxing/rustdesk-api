package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// Node 集群节点管理（后台）。
//
// 职责：查看节点状态与负载、调整调度权重、把节点下线/上线、删除节点。
// 节点本身通过 `/api/node/*` 注册与心跳，这里不依赖节点可主动被连。
type Node struct{}

// List 节点列表
// @Tags 集群节点
// @Summary 节点列表
// @Router /admin/node/list [get]
// @Security token
func (n *Node) List(c *gin.Context) {
	kind := c.Query("kind")
	nodes := service.AllService.NodeService.List(kind)
	response.Success(c, gin.H{
		"nodes":      nodes,
		"weights":    service.AllService.NodeService.Weights(),
		"offlineSec": service.AllService.NodeService.OfflineSec(),
	})
}

// Update 更新节点属性（权限：管理员）
// @Tags 集群节点
// @Summary 更新节点（权重/地域/上限/下线开关）
// @Router /admin/node/update [post]
// @Security token
func (n *Node) Update(c *gin.Context) {
	type Req struct {
		Id          uint    `json:"id" binding:"required"`
		Name        *string `json:"name"`
		Addr        *string `json:"addr"`
		Region      *string `json:"region"`
		LanAddr     *string `json:"lan_addr"`
		Weight      *int    `json:"weight"`
		MaxSessions *int    `json:"max_sessions"`
		Disabled    *int    `json:"disabled"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	var node model.ServerNode
	if err := service.DB.Where("id = ?", req.Id).First(&node).Error; err != nil {
		response.Fail(c, 101, "节点不存在")
		return
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Addr != nil && *req.Addr != "" {
		// 允许后台修正节点对外地址（例如运营商标识变化、公网 IP 写错）
		updates["addr"] = *req.Addr
	}
	if req.Region != nil {
		// 与客户端使用同一套省份 code，避免"节点填了别的值、永远匹配不上"
		if !service.ValidProvinceCode(*req.Region) {
			response.Fail(c, 101, "非法地域代码（应为省份代码，如 GD/GX；留空表示不设置）")
			return
		}
		updates["region"] = *req.Region
	}
	if req.LanAddr != nil {
		updates["lan_addr"] = *req.LanAddr
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if req.MaxSessions != nil {
		updates["max_sessions"] = *req.MaxSessions
	}
	if req.Disabled != nil {
		updates["disabled"] = *req.Disabled
	}
	if len(updates) == 0 {
		response.Fail(c, 101, "没有要更新的字段")
		return
	}
	if err := service.DB.Model(&model.ServerNode{}).Where("id = ?", req.Id).Updates(updates).Error; err != nil {
		response.Fail(c, 101, "更新失败："+err.Error())
		return
	}
	global.Logger.Infof("[Node] 管理员更新节点 %s: %v", node.NodeId, updates)
	response.Success(c, gin.H{"ok": true})
}

// Delete 删除节点
// @Tags 集群节点
// @Summary 删除节点
// @Router /admin/node/delete [post]
// @Security token
func (n *Node) Delete(c *gin.Context) {
	type Req struct {
		Id uint `json:"id" binding:"required"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	var node model.ServerNode
	if err := service.DB.Where("id = ?", req.Id).First(&node).Error; err != nil {
		response.Fail(c, 101, "节点不存在")
		return
	}
	if err := service.DB.Delete(&model.ServerNode{}, req.Id).Error; err != nil {
		response.Fail(c, 101, "删除失败："+err.Error())
		return
	}
	global.Logger.Infof("[Node] 管理员删除节点 %s", node.NodeId)
	response.Success(c, gin.H{"ok": true})
}

// ScoreConfig 读取/保存调度权重
// @Tags 集群节点
// @Summary 调度权重（多因子加权）
// @Router /admin/node/score-config [get]
// @Security token
func (n *Node) ScoreConfig(c *gin.Context) {
	response.Success(c, gin.H{
		"weights": service.AllService.NodeService.Weights(),
		"default": service.DefaultNodeScoreWeights(),
	})
}

// SaveScoreConfig 保存调度权重
// @Tags 集群节点
// @Summary 保存调度权重
// @Router /admin/node/score-config [post]
// @Security token
func (n *Node) SaveScoreConfig(c *gin.Context) {
	var w service.NodeScoreWeights
	if err := c.ShouldBindJSON(&w); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	// 权重允许 0~1；负数或过大视为非法，避免把排序搞乱。
	v := []float64{w.Sessions, w.Bandwidth, w.CPU, w.Mem, w.RTT, w.Weight}
	for _, x := range v {
		if x < 0 || x > 1 {
			response.Fail(c, 101, "权重需在 0~1 之间")
			return
		}
	}
	if err := service.AllService.NodeService.SaveWeights(w); err != nil {
		response.Fail(c, 101, "保存失败："+err.Error())
		return
	}
	global.Logger.Infof("[Node] 调度权重已更新: %+v", w)
	response.Success(c, gin.H{"ok": true})
}

// Refresh 立即重新探测所有节点（回填 RTT / 在线状态）
// @Tags 集群节点
// @Summary 立即刷新节点状态
// @Router /admin/node/refresh [post]
// @Security token
func (n *Node) Refresh(c *gin.Context) {
	nodes := service.AllService.NodeService.List("")
	for i := range nodes {
		service.AllService.NodeService.Probe(&nodes[i])
	}
	response.Success(c, gin.H{"ok": true, "count": len(nodes)})
}

// StaticRelays 读取静态中继兜底列表
// @Tags 集群节点
// @Summary 静态中继兜底列表
// @Router /admin/node/static-relays [get]
// @Security token
func (n *Node) StaticRelays(c *gin.Context) {
	response.Success(c, gin.H{"list": service.AllService.NodeService.StaticRelays()})
}

// SaveStaticRelays 保存静态中继兜底列表
// @Tags 集群节点
// @Summary 保存静态中继兜底列表
// @Router /admin/node/static-relays [post]
// @Security token
func (n *Node) SaveStaticRelays(c *gin.Context) {
	type Req struct {
		List []string `json:"list"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	for _, a := range req.List {
		if a == "" {
			response.Fail(c, 101, "地址不能为空")
			return
		}
	}
	if err := service.AllService.NodeService.SaveStaticRelays(req.List); err != nil {
		response.Fail(c, 101, "保存失败："+err.Error())
		return
	}
	global.Logger.Infof("[Node] 静态中继已更新: %v", req.List)
	response.Success(c, gin.H{"ok": true})
}