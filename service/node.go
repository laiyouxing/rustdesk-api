package service

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

// 集群节点相关设置键
const (
	// SettingKeyNodeScoreWeights 调度权重（JSON）
	SettingKeyNodeScoreWeights = "node.score_weights"
	// SettingKeyNodeOfflineSec 心跳超时秒数，超过即判定离线
	SettingKeyNodeOfflineSec = "node.offline_sec"
)

// NodeScoreWeights 多因子调度的权重。
//
// 设计：各项先归一化到 0~1，再按权重加权求和，**分数越低越优先**。
// 管理员可在后台调整权重，改动立即影响下一次下发的中继顺序。
type NodeScoreWeights struct {
	Sessions  float64 `json:"sessions"`  // 会话数 / 上限
	Bandwidth float64 `json:"bandwidth"` // (上行+下行) / 总带宽
	CPU       float64 `json:"cpu"`       // CPU 占用
	Mem       float64 `json:"mem"`       // 内存占用
	RTT       float64 `json:"rtt"`       // api-server 到节点的探测延迟
	Weight    float64 `json:"weight"`    // 人工权重（越大越优先，做负向）
}

// DefaultNodeScoreWeights 默认权重：
// 以负载为主（会话+带宽占 55%），资源占用 25%，延迟 20%，人工权重作为微调。
func DefaultNodeScoreWeights() NodeScoreWeights {
	return NodeScoreWeights{
		Sessions:  0.35,
		Bandwidth: 0.20,
		CPU:       0.15,
		Mem:       0.10,
		RTT:       0.20,
		Weight:    0.0,
	}
}

// NodeService 集群节点注册、心跳、健康检查与调度评分。
type NodeService struct {
}

// NodeRegisterForm 节点注册参数
type NodeRegisterForm struct {
	NodeId      string `json:"node_id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Addr        string `json:"addr"`
	LanAddr     string `json:"lan_addr"`
	Region      string `json:"region"`
	Version     string `json:"version"`
	MaxSessions int    `json:"max_sessions"`
	Weight      int    `json:"weight"`
}

// NodeHeartbeatForm 节点心跳参数
type NodeHeartbeatForm struct {
	NodeId    string `json:"node_id"`
	Peers     int64  `json:"peers"`
	Sessions  int64  `json:"sessions"`
	BwIn      int64  `json:"bw_in"`
	BwOut     int64  `json:"bw_out"`
	CpuPct    int    `json:"cpu_pct"`
	MemPct    int    `json:"mem_pct"`
	UptimeSec int64  `json:"uptime_sec"`
	Version   string `json:"version"`
}

// Register 节点注册（幂等：已存在则更新静态信息，不覆盖管理员改过的 weight/region）。
func (s *NodeService) Register(f *NodeRegisterForm) (*model.ServerNode, error) {
	if f.NodeId == "" || f.Addr == "" || f.Kind == "" {
		return nil, fmt.Errorf("node_id/kind/addr 不能为空")
	}
	var node model.ServerNode
	err := DB.Where(&model.ServerNode{NodeId: f.NodeId}).First(&node).Error
	if err != nil {
		// 新节点
		node = model.ServerNode{
			NodeId:      f.NodeId,
			Kind:        f.Kind,
			Name:        f.Name,
			Addr:        f.Addr,
			LanAddr:     f.LanAddr,
			Region:      f.Region,
			Version:     f.Version,
			Weight:      f.Weight,
			MaxSessions: f.MaxSessions,
			Status:      1,
			LastSeen:    time.Now().Unix(),
		}
		if node.Weight == 0 {
			node.Weight = 100
		}
		if err2 := DB.Create(&node).Error; err2 != nil {
			return nil, err2
		}
		global.Logger.Infof("[Node] 新节点注册：%s kind=%s addr=%s region=%s", node.NodeId, node.Kind, node.Addr, node.Region)
		return &node, nil
	}
	// 已存在：只更新由节点自身上报的字段；weight/region/disabled 由管理员掌控，
	// 仅当节点显式带值且当前为空时才补。
	updates := map[string]interface{}{
		"addr":      f.Addr,
		"version":   f.Version,
		"status":    1,
		"last_seen": time.Now().Unix(),
	}
	if f.LanAddr != "" {
		updates["lan_addr"] = f.LanAddr
	}
	if f.Name != "" {
		updates["name"] = f.Name
	}
	if f.MaxSessions > 0 {
		updates["max_sessions"] = f.MaxSessions
	}
	if node.Region == "" && f.Region != "" {
		updates["region"] = f.Region
	}
	if err := DB.Model(&model.ServerNode{}).Where("id = ?", node.Id).Updates(updates).Error; err != nil {
		return nil, err
	}
	global.Logger.Infof("[Node] 节点重新注册：%s addr=%s", node.NodeId, f.Addr)
	DB.Where("id = ?", node.Id).First(&node)
	return &node, nil
}

// Heartbeat 记录节点心跳与实时指标。
func (s *NodeService) Heartbeat(f *NodeHeartbeatForm) error {
	if f.NodeId == "" {
		return fmt.Errorf("node_id 不能为空")
	}
	updates := map[string]interface{}{
		"peers":      f.Peers,
		"sessions":   f.Sessions,
		"bw_in":      f.BwIn,
		"bw_out":     f.BwOut,
		"cpu_pct":    f.CpuPct,
		"mem_pct":    f.MemPct,
		"uptime_sec": f.UptimeSec,
		"status":     1,
		"last_seen":  time.Now().Unix(),
	}
	if f.Version != "" {
		updates["version"] = f.Version
	}
	res := DB.Model(&model.ServerNode{}).Where("node_id = ?", f.NodeId).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("节点未注册：%s", f.NodeId)
	}
	return nil
}

// OfflineSec 心跳超时秒数（默认 60 秒）。
func (s *NodeService) OfflineSec() int64 {
	if n, ok := AllService.SettingService.GetInt(SettingKeyNodeOfflineSec); ok && n > 0 {
		return int64(n)
	}
	return 60
}

// Weights 读取调度权重；未配置时返回默认值。
func (s *NodeService) Weights() NodeScoreWeights {
	if v, ok := AllService.SettingService.Get(SettingKeyNodeScoreWeights); ok && v != "" {
		var w NodeScoreWeights
		if err := json.Unmarshal([]byte(v), &w); err == nil {
			return w
		}
	}
	return DefaultNodeScoreWeights()
}

// SaveWeights 保存调度权重。
func (s *NodeService) SaveWeights(w NodeScoreWeights) error {
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	return AllService.SettingService.Set(SettingKeyNodeScoreWeights, string(b))
}

// List 返回所有节点（可按 kind 过滤）。
func (s *NodeService) List(kind string) []model.ServerNode {
	var nodes []model.ServerNode
	q := DB.Model(&model.ServerNode{})
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	q.Order("region asc, id asc").Find(&nodes)
	// 顺带回填在线状态，避免前端看到过期状态
	offline := s.OfflineSec()
	now := time.Now().Unix()
	for i := range nodes {
		if nodes[i].LastSeen == 0 || now-nodes[i].LastSeen > offline {
			nodes[i].Status = 0
		}
	}
	return nodes
}

// normalize 把 value/max 归一化到 0~1（max<=0 时按固定参考值）。
func normalize(value, max float64) float64 {
	if max <= 0 {
		return 0
	}
	r := value / max
	if r < 0 {
		return 0
	}
	if r > 1 {
		return 1
	}
	return r
}

// Score 计算单个节点的调度分数（越小越优先）。
func (s *NodeService) Score(n *model.ServerNode, w NodeScoreWeights) float64 {
	// 会话：有上限按上限归一化，没有上限用 1000 作为参考容量
	maxSess := float64(n.MaxSessions)
	if maxSess <= 0 {
		maxSess = 1000
	}
	sSess := normalize(float64(n.Sessions), maxSess)
	// 带宽：按 1 Gbit/s 总带宽参考
	sBw := normalize(float64(n.BwIn+n.BwOut), 1024*1024*1024)
	sCpu := normalize(float64(n.CpuPct), 100)
	sMem := normalize(float64(n.MemPct), 100)
	sRtt := normalize(float64(n.RttMs), 500)
	// 人工权重：越大越优先 → 分数越低
	sWeight := normalize(float64(n.Weight), 100)

	score := w.Sessions*sSess + w.Bandwidth*sBw + w.CPU*sCpu + w.Mem*sMem + w.RTT*sRtt - w.Weight*sWeight
	if score < -10 {
		score = -10
	}
	return score
}

// Relays 返回按调度分数排序的可用中继列表（供 hbbs 拉取，或直接下发给客户端）。
//
// 只包含：kind=hbbr、在线、未被管理员下线。
func (s *NodeService) Relays(region string, limit int) []map[string]interface{} {
	nodes := s.List("hbbr")
	w := s.Weights()
	offline := s.OfflineSec()
	now := time.Now().Unix()

	type scored struct {
		node  model.ServerNode
		score float64
	}
	list := make([]scored, 0, len(nodes))
	for _, n := range nodes {
		if n.Disabled == 1 {
			continue
		}
		if n.LastSeen == 0 || now-n.LastSeen > offline {
			continue
		}
		sc := s.Score(&n, w)
		list = append(list, scored{node: n, score: sc})
		// 顺手把分数写回，便于前端展示
		DB.Model(&model.ServerNode{}).Where("id = ?", n.Id).Update("score", sc)
	}
	sort.Slice(list, func(i, j int) bool {
		// 指定了 region 且能匹配时，优先同地域
		if region != "" {
			ri := list[i].node.Region == region
			rj := list[j].node.Region == region
			if ri != rj {
				return ri
			}
		}
		return list[i].score < list[j].score
	})
	out := make([]map[string]interface{}, 0, len(list))
	for i, it := range list {
		if limit > 0 && i >= limit {
			break
		}
		item := map[string]interface{}{
			"addr":     it.node.Addr,
			"score":    it.score,
			"region":   it.node.Region,
			"sessions": it.node.Sessions,
			"weight":   it.node.Weight,
		}
		if it.node.LanAddr != "" {
			item["lan_addr"] = it.node.LanAddr
		}
		out = append(out, item)
	}
	return out
}

// TouchRtt 健康检查时回填 api-server 到该节点的 TCP 建连耗时。
func (s *NodeService) TouchRtt(id uint, rttMs int) {
	DB.Model(&model.ServerNode{}).Where("id = ?", id).Update("rtt_ms", rttMs)
}

// Probe 探测节点 TCP 可达性并回填 RTT。
func (s *NodeService) Probe(n *model.ServerNode) {
	begin := time.Now()
	conn, err := net.DialTimeout("tcp", n.Addr, 1200*time.Millisecond)
	if err != nil {
		DB.Model(&model.ServerNode{}).Where("id = ?", n.Id).Updates(map[string]interface{}{
			"rtt_ms": 0,
			"status": 0,
		})
		return
	}
	_ = conn.Close()
	s.TouchRtt(n.Id, int(time.Since(begin).Milliseconds()))
}

// StartHealthCheck 定时健康检查：探测可达性、判定离线。
func (s *NodeService) StartHealthCheck() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		nodes := s.List("")
		for i := range nodes {
			s.Probe(&nodes[i])
		}
	}
}
