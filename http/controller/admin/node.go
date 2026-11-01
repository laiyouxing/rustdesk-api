package admin

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

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

// randomNodeSecret 生成节点专用密钥（24 字节随机数 → 48 位 hex）。
func randomNodeSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

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
	lockedFields := []string{}
	unlockedFields := []string{}

	// 只有值**确实变化**时才写入/登记接管。
	// 否则前端每次保存都携带 name/region/lan_addr，会把没改过的字段也锁死。
	if req.Name != nil && *req.Name != node.Name {
		updates["name"] = *req.Name
		if *req.Name == "" {
			unlockedFields = append(unlockedFields, "name")
		} else {
			lockedFields = append(lockedFields, "name")
		}
	}
	if req.Addr != nil && strings.TrimSpace(*req.Addr) != "" {
		// 允许后台修正节点对外地址（例如运营商标识变化、公网 IP 写错）
		addr := strings.TrimSpace(*req.Addr)
		if !service.ValidHostPort(addr) {
			response.Fail(c, 101, "地址格式不正确，应为 host:port（如 1.2.3.4:21117）")
			return
		}
		if addr != node.Addr {
			updates["addr"] = addr
			lockedFields = append(lockedFields, "addr")
		}
	}
	if req.Region != nil {
		// 与客户端使用同一套省份 code，避免"节点填了别的值、永远匹配不上"
		if !service.ValidProvinceCode(*req.Region) {
			response.Fail(c, 101, "非法地域代码（应为省份代码，如 GD/GX；留空表示不设置）")
			return
		}
		if *req.Region != node.Region {
			updates["region"] = *req.Region
			if *req.Region == "" {
				// 清空地域 = 交还给节点上报，同时解除接管
				unlockedFields = append(unlockedFields, "region")
			} else {
				lockedFields = append(lockedFields, "region")
			}
		}
	}
	if req.LanAddr != nil {
		lan := strings.TrimSpace(*req.LanAddr)
		if lan != "" && !service.ValidHostPort(lan) {
			response.Fail(c, 101, "内网地址格式不正确，应为 host:port（如 10.126.126.5:21117）")
			return
		}
		if lan != node.LanAddr {
			updates["lan_addr"] = lan
			if lan == "" {
				unlockedFields = append(unlockedFields, "lan_addr")
			} else {
				lockedFields = append(lockedFields, "lan_addr")
			}
		}
	}
	if req.Weight != nil {
		if *req.Weight < 0 {
			response.Fail(c, 101, "人工权重不能为负数")
			return
		}
		if *req.Weight != node.Weight {
			updates["weight"] = *req.Weight
		}
	}
	if req.MaxSessions != nil {
		if *req.MaxSessions < 0 {
			response.Fail(c, 101, "会话上限不能为负数")
			return
		}
		if *req.MaxSessions != node.MaxSessions {
			updates["max_sessions"] = *req.MaxSessions
			if *req.MaxSessions > 0 {
				lockedFields = append(lockedFields, "max_sessions")
			} else {
				unlockedFields = append(unlockedFields, "max_sessions")
			}
		}
	}
	if req.Disabled != nil {
		if *req.Disabled != 0 && *req.Disabled != 1 {
			response.Fail(c, 101, "disabled 只能是 0 或 1")
			return
		}
		if *req.Disabled != node.Disabled {
			updates["disabled"] = *req.Disabled
		}
	}
	if len(updates) == 0 && len(lockedFields) == 0 && len(unlockedFields) == 0 {
		response.Fail(c, 101, "没有要更新的字段")
		return
	}
	if len(updates) > 0 {
		if err := service.DB.Model(&model.ServerNode{}).Where("id = ?", req.Id).Updates(updates).Error; err != nil {
			response.Fail(c, 101, "更新失败："+err.Error())
			return
		}
	}
	// 记录平台接管的字段：这些字段以后不再被节点注册上报值覆盖（平台优先）
	if err := service.AllService.NodeService.MarkOverrides(req.Id, lockedFields); err != nil {
		global.Logger.Warnf("[Node] 记录接管字段失败 id=%d: %v", req.Id, err)
	}
	// 清空字段时自动解除接管，避免"单行道"：管理员清空后节点可以重新上报。
	// 注意必须判空：UnlockOverrides 的语义是"fields 为空 = 解除全部"，
	// 无脑调用会把管理员之前设置的接管全部清掉。
	if len(unlockedFields) > 0 {
		if err := service.AllService.NodeService.UnlockOverrides(req.Id, unlockedFields); err != nil {
			global.Logger.Warnf("[Node] 解除接管字段失败 id=%d: %v", req.Id, err)
		}
	}
	global.Logger.Infof("[Node] 管理员更新节点 %s: %v（新增接管 %v，解除接管 %v）", node.NodeId, updates, lockedFields, unlockedFields)
	response.Success(c, gin.H{"ok": true, "locked": lockedFields, "unlocked": unlockedFields})
}

// Unlock 解除平台接管（fields 为空表示全部解除）
// @Tags 集群节点
// @Summary 解除平台接管字段
// @Router /admin/node/unlock [post]
// @Security token
func (n *Node) Unlock(c *gin.Context) {
	type Req struct {
		Id     uint     `json:"id" binding:"required"`
		Fields []string `json:"fields"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if err := service.AllService.NodeService.UnlockOverrides(req.Id, req.Fields); err != nil {
		response.Fail(c, 101, "解除接管失败："+err.Error())
		return
	}
	global.Logger.Infof("[Node] 管理员解除节点接管 id=%d fields=%v", req.Id, req.Fields)
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

// Refresh 立即重新探测所有节点（回填 RTT）
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
	// 去空白 / 去重 / host:port 校验在 service 层统一做
	if err := service.AllService.NodeService.SaveStaticRelays(req.List); err != nil {
		response.Fail(c, 101, "保存失败："+err.Error())
		return
	}
	global.Logger.Infof("[Node] 静态中继已更新: %v", req.List)
	response.Success(c, gin.H{"ok": true})
}

// GetSecret 读取节点专用密钥与轮换状态
// @Tags 集群节点
// @Summary 读取节点接入密钥
// @Router /admin/node/secret [get]
// @Security token
func (n *Node) GetSecret(c *gin.Context) {
	secret := service.AllService.NodeService.NodeSecret()
	rotatedAt := service.AllService.NodeService.NodeSecretRotatedAt()
	inGrace := service.AllService.NodeService.NodeSecretPrevInGrace()
	expiresAt := int64(0)
	if inGrace {
		expiresAt = service.AllService.NodeService.NodeSecretGraceDeadline()
	}
	response.Success(c, gin.H{
		"secret":          secret,
		"rotated_at":      rotatedAt,
		"has_prev":        inGrace,
		"prev_expires_at": expiresAt,
		"grace_seconds":   service.NodeSecretGraceSeconds,
	})
}

// SaveSecret 轮换节点专用密钥（传空串则由服务端生成随机密钥并保存）
// @Tags 集群节点
// @Summary 轮换节点接入密钥
// @Router /admin/node/secret [post]
// @Security token
func (n *Node) SaveSecret(c *gin.Context) {
	type Req struct {
		Secret string `json:"secret"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		generated, err := randomNodeSecret()
		if err != nil {
			response.Fail(c, 101, "生成密钥失败："+err.Error())
			return
		}
		secret = generated
	}
	if len(secret) < 16 {
		response.Fail(c, 101, "节点密钥长度至少 16 位")
		return
	}
	rotatedAt, err := service.AllService.NodeService.RotateNodeSecret(secret)
	if err != nil {
		response.Fail(c, 101, "保存失败："+err.Error())
		return
	}
	// 不要记录密钥内容本身
	global.Logger.Infof("[Node] 节点接入密钥已轮换（长度 %d，宽限期 %d 天），旧密钥保留至 %d",
		len(secret), service.NodeSecretGraceSeconds/86400, rotatedAt+service.NodeSecretGraceSeconds)
	response.Success(c, gin.H{"secret": secret, "rotated_at": rotatedAt})
}

// GetConfig 读取节点当前下发的配置
// @Tags 集群节点
// @Summary 读取节点配置
// @Router /admin/node/config [get]
// @Security token
func (n *Node) GetConfig(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		response.Fail(c, 101, "id 不能为空")
		return
	}
	var node model.ServerNode
	if err := service.DB.Where("id = ?", id).First(&node).Error; err != nil {
		response.Fail(c, 101, "节点不存在")
		return
	}
	response.Success(c, gin.H{"config": node.Config, "version": node.ConfigVersion, "node_id": node.NodeId})
}

// SaveConfig 下发节点配置（JSON）
//
// 应用说明（后台页面会提示）：
//   - 可热应用：relay_servers（hbbs 中继列表）、limit_speed_mbps /
//     single_bandwidth_mbps / total_bandwidth_mbps / downgrade_threshold /
//     downgrade_start_check_sec（hbbr 限速与降级）
//   - 需重启：key / port / mask / must_login —— 属于启动参数，仅做展示
//
// @Tags 集群节点
// @Summary 下发节点配置
// @Router /admin/node/config [post]
// @Security token
func (n *Node) SaveConfig(c *gin.Context) {
	type Req struct {
		Id     uint   `json:"id" binding:"required"`
		Config string `json:"config"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	ver, err := service.AllService.NodeService.SaveConfig(req.Id, req.Config)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	global.Logger.Infof("[Node] 已下发配置到节点 id=%d（版本 %d）", req.Id, ver)
	response.Success(c, gin.H{"ok": true, "version": ver})
}
