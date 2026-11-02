package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

// 集群节点相关设置键
const (
	// SettingKeyNodeScoreWeights 调度权重（JSON）
	SettingKeyNodeScoreWeights = "node.score_weights"
	// SettingKeyNodeOfflineSec 心跳超时秒数，超过即判定离线
	SettingKeyNodeOfflineSec = "node.offline_sec"
	// SettingKeyNodeStaticRelays 静态中继（逗号分隔）。
	//
	// 背景：调度列表只包含"已注册且在线"的节点。集群扩容过渡期内、或存在
	// 未接入 agent 的中继时，若直接用调度结果**替换**静态 -r，会把中继数量
	// 缩减（例如 3 台变 1 台），反而降低可用性。这里改为**合并**：调度结果
	// 在前（按分数排序），静态中继补在后面兜底。
	SettingKeyNodeStaticRelays = "node.static_relays"
	// SettingKeyNodeSecret 节点专用密钥（请求头 X-Node-Secret）。
	//
	// ProcessMonitorAuth 接受的 rustdesk key 是 hbbs 公钥，会下发给每个客户端，
	// 等于公开信息；仅靠它无法阻止任何人伪造节点。节点写接口额外校验这个
	// 专用密钥；未配置时告警并放行（兼容升级）。
	SettingKeyNodeSecret = "node.secret"
	// SettingKeyNodeSecretPrev 上一把节点密钥（轮换后的宽限期用）。
	//
	// 轮换瞬间已在线的节点仍然只有旧密钥，若立刻只认新密钥会让它们全部 401
	// 掉线；旧密钥保留一段宽限期（NodeSecretGraceSeconds），期间新老都接受，
	// 节点会通过 GET /api/node/secret 自动换到新密钥。
	SettingKeyNodeSecretPrev = "node.secret_prev"
	// SettingKeyNodeSecretRotatedAt 最近一次轮换时间（unix 秒），用于计算宽限期。
	SettingKeyNodeSecretRotatedAt = "node.secret_rotated_at"
)

// NodeSecretGraceSeconds 旧密钥宽限期：7 天。
//
// 节点每 60 秒轮询一次 GET /api/node/secret，7 天足够覆盖升级/重启窗口；
// 到期后旧密钥自动失效，无需额外的清理任务。
const NodeSecretGraceSeconds int64 = 7 * 24 * 3600

// maxNodeConfigBytes 节点配置 JSON 的大小上限（64KB）。
const maxNodeConfigBytes = 64 << 10

// validNodeKinds 允许注册的节点类型。
var validNodeKinds = map[string]bool{"hbbs": true, "hbbr": true}

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
	Weight    float64 `json:"weight"`    // 人工权重（越大越优先，折算为更低的成本）
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
	NodeId  string `json:"node_id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Addr    string `json:"addr"`
	LanAddr string `json:"lan_addr"`
	// LanAddrs 用指针区分「未发送」与「空数组」：
	//   - nil     = 本次注册没带这个字段（旧版 agent），**不动**库里已有的候选；
	//   - 空数组  = 节点明确上报"没有任何可用内网地址"，把 lan_addrs 与 lan_addr 一起清空。
	// 用 []string 时两者都解码成 nil，历史数据里的候选会被"没带字段"悄悄清掉。
	LanAddrs    *[]string `json:"lan_addrs"`
	Region      string    `json:"region"`
	Version     string    `json:"version"`
	MaxSessions int       `json:"max_sessions"`
	Weight      int       `json:"weight"`
}

// NodeHeartbeatForm 节点心跳参数
//
// 注意：这里**没有** lan_addrs 字段。节点 agent 只在注册时上报候选内网地址，
// 心跳报文里从来不带（此前保留字段 + 刷新分支等于一段永远走不到的死代码）。
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

// maxLanAddrs 候选内网地址上限：多网卡也就几张，超过说明数据有问题。
const maxLanAddrs = 8

// maxLanAddrLen 单条候选内网地址长度上限（IPv4 host:port 最长也就 21 字符）。
const maxLanAddrLen = 64

// isPrivateIPv4 判断 IP 是否属于私网/内网网段。
//
// 只认这四段：10/8、172.16/12、192.168/16、100.64/10（运营商级 NAT，部分 VPN
// 也用这一段）。公网地址不能当"内网候选"：客户端拿到后会去直连一个公网地址，
// 既省不了绕行，又会让 relay-server-lan 的择优逻辑白跑一次探测。
func isPrivateIPv4(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	switch {
	case v4[0] == 10: // 10.0.0.0/8
		return true
	case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31: // 172.16.0.0/12
		return true
	case v4[0] == 192 && v4[1] == 168: // 192.168.0.0/16
		return true
	case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // 100.64.0.0/10
		return true
	}
	return false
}

// IsPrivateLanAddr 校验一条候选内网地址：合法 host:port、长度不超限、主机部分是私网 IPv4。
//
// 后台编辑接口用它先给出明确报错（管理员填错不该被静默丢弃）；
// normalizeLanAddrs 用它做节点上报数据的静默过滤，两处规则完全一致。
func IsPrivateLanAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" || len(addr) > maxLanAddrLen || !ValidHostPort(addr) {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && isPrivateIPv4(ip)
}

// normalizeLanAddrs 清洗节点上报的候选内网地址。
//
// 保留私网地址（这正是要的），只丢掉不合规的值：空串、长度超限、非法 host:port、
// 主机部分不是私网 IPv4（回环 / 公网 / IPv6 / 容器外的脏数据）。
// 不合规项**静默丢弃**：这些是节点按本机网卡自动探测出来的，为一条脏数据整体
// 报错会让节点彻底注册不上。
// 去重并保序（第一个作为兜底的内网地址），最多留 maxLanAddrs 个。
func normalizeLanAddrs(list []string) []string {
	out := make([]string, 0, len(list))
	seen := make(map[string]bool, len(list))
	for _, a := range list {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] || !IsPrivateLanAddr(a) {
			continue
		}
		seen[a] = true
		out = append(out, a)
		if len(out) >= maxLanAddrs {
			break
		}
	}
	return out
}

// NormalizeLanAddrs 候选内网地址清洗的导出包装（后台编辑接口复用）。
func NormalizeLanAddrs(list []string) []string { return normalizeLanAddrs(list) }

// splitLanAddrs 把库里的逗号分隔串还原成数组（给探测/后台展示用）。
func splitLanAddrs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseOverrideList 解析 Overrides 字段（JSON 字符串数组）。
//
// 内容损坏时**告警**而不是静默忽略：静默会让"平台接管保护"悄悄失效，
// 管理员在后台改的值下一轮注册就被节点冲回去，且没有任何线索。
func parseOverrideList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	list := []string{}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		global.Logger.Warnf("[Node] overrides 内容损坏，按未接管处理（平台保护可能失效）：%q err=%v", raw, err)
		return nil
	}
	return list
}

func overrideSet(list []string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, k := range list {
		if k != "" {
			set[k] = true
		}
	}
	return set
}

// Register 节点注册（幂等：已存在则更新静态信息，不覆盖平台已接管的字段）。
func (s *NodeService) Register(f *NodeRegisterForm) (*model.ServerNode, error) {
	// 地域必须与客户端使用同一套省份 code，否则永远匹配不上；
	// 非法值不阻断注册（可能是环境变量写错，不该让节点失联），降级为空并告警。
	if f.Region != "" && !ValidProvinceCode(f.Region) {
		global.Logger.Warnf("[Node] 节点 %s 上报了非法地域 %q，已忽略（应使用省份 code，如 GD/GX）", f.NodeId, f.Region)
		f.Region = ""
	}
	f.Kind = strings.TrimSpace(f.Kind)
	f.NodeId = strings.TrimSpace(f.NodeId)
	f.Addr = strings.TrimSpace(f.Addr)
	f.LanAddr = strings.TrimSpace(f.LanAddr)
	// 候选内网地址：nil（未发送）与空数组的语义不同 —— 前者是旧版 agent
	// 只是没带这个字段，不能把库里已有的候选清掉；后者是节点明确上报
	// "我一个内网地址都没有"，应当把 lan_addrs / lan_addr 一起清空。
	lanAddrsSent := f.LanAddrs != nil
	var lanList []string
	if lanAddrsSent {
		lanList = normalizeLanAddrs(*f.LanAddrs)
		// 有列表时让旧的单值字段与第一个保持一致，
		// 避免"lan_addr 与 lan_addrs[0] 不一致"导致探测和展示各说各话。
		if len(lanList) > 0 {
			f.LanAddr = lanList[0]
		}
	}
	if f.NodeId == "" || f.Addr == "" || f.Kind == "" {
		return nil, fmt.Errorf("node_id/kind/addr 不能为空")
	}
	if !validNodeKinds[f.Kind] {
		return nil, fmt.Errorf("非法节点类型 kind=%q（只支持 hbbs / hbbr）", f.Kind)
	}

	var node model.ServerNode
	found := true
	if err := DB.Where(&model.ServerNode{NodeId: f.NodeId}).First(&node).Error; err != nil {
		found = false
		// node_id 约定为 `{kind}@{addr}`，而 addr 未显式配置时是节点自动探测的
		// 公网 IP：IP 一变 node_id 就变，若直接建新行，旧行会变成永远离线的
		// 幽灵记录，且平台配置（config/overrides/weight）全部留在旧行上丢失。
		// 这里回退用 `kind + lan_addr` 识别同一台物理机：lan_addr 由部署者手工
		// 指定，比公网 IP 稳定；只有唯一命中时才复用，避免误合并。
		if f.LanAddr != "" {
			candidates := make([]model.ServerNode, 0, 2)
			if e := DB.Where("kind = ? AND lan_addr = ?", f.Kind, f.LanAddr).Limit(2).Find(&candidates).Error; e == nil && len(candidates) == 1 {
				node = candidates[0]
				found = true
				global.Logger.Infof("[Node] node_id 未命中（%s），按 kind+lan_addr 复用已有节点 id=%d old_node_id=%s",
					f.NodeId, node.Id, node.NodeId)
			}
		}
	}

	if !found {
		// 新节点
		node = model.ServerNode{
			NodeId:      f.NodeId,
			Kind:        f.Kind,
			Name:        f.Name,
			Addr:        f.Addr,
			LanAddr:     f.LanAddr,
			LanAddrs:    strings.Join(lanList, ","),
			Region:      f.Region,
			Version:     f.Version,
			Weight:      f.Weight,
			MaxSessions: f.MaxSessions,
			Status:      1,
			// -1 = 未知/探测失败：节点刚注册，健康检查还没探测过。
			// 不能用 0，否则会被评分当成"0ms 最优"。
			RttMs:    -1,
			LastSeen: time.Now().Unix(),
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

	// 已存在（或按 lan_addr 复用）：只更新由节点自身上报的字段；
	// weight/region/disabled 由管理员掌控，仅当节点显式带值且当前为空时才补。
	// 平台已接管的字段不再被节点上报覆盖（平台优先）。
	locked := overrideSet(parseOverrideList(node.Overrides))
	updates := map[string]interface{}{
		"version":   f.Version,
		"status":    1,
		"last_seen": time.Now().Unix(),
	}
	if f.Addr != "" && !locked["addr"] {
		updates["addr"] = f.Addr
	}
	if f.LanAddr != "" && !locked["lan_addr"] {
		updates["lan_addr"] = f.LanAddr
	}
	// 候选列表整体覆盖：网卡增减时节点每轮上报的都是全量。
	// 显式空数组 = 节点确认没有可用内网地址，两个字段一起清掉；否则旧的
	// lan_addr 会一直留着，客户端继续去连一个已经不存在的地址。
	if lanAddrsSent && !locked["lan_addr"] {
		if len(lanList) > 0 {
			updates["lan_addrs"] = strings.Join(lanList, ",")
		} else {
			updates["lan_addrs"] = ""
			updates["lan_addr"] = ""
		}
	}
	if f.Name != "" && !locked["name"] {
		updates["name"] = f.Name
	}
	if f.MaxSessions > 0 && !locked["max_sessions"] {
		updates["max_sessions"] = f.MaxSessions
	}
	if node.Region == "" && f.Region != "" && !locked["region"] {
		updates["region"] = f.Region
	}
	// 按 lan_addr 复用时 node_id 仍是旧的 `{kind}@{旧addr}`，必须同步成新值；
	// 否则唯一索引里留着旧标识，下次注册又要走一遍回退匹配。
	if node.NodeId != f.NodeId {
		updates["node_id"] = f.NodeId
	}
	if err := DB.Model(&model.ServerNode{}).Where("id = ?", node.Id).Updates(updates).Error; err != nil {
		return nil, err
	}
	global.Logger.Infof("[Node] 节点重新注册：%s addr=%s", f.NodeId, f.Addr)
	DB.Where("id = ?", node.Id).First(&node)
	return &node, nil
}

// Heartbeat 记录节点心跳与实时指标。
func (s *NodeService) Heartbeat(f *NodeHeartbeatForm) error {
	if f.NodeId == "" {
		return fmt.Errorf("node_id 不能为空")
	}
	// 先显式判断节点是否存在：不能用 RowsAffected==0 判断。
	// MySQL 默认返回"实际变更行数"，同一秒内两次心跳的值完全相同会被判成 0，
	// 进而误报"节点未注册"，让 agent 反复重新注册。
	var cnt int64
	if err := DB.Model(&model.ServerNode{}).Where("node_id = ?", f.NodeId).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return fmt.Errorf("节点未注册：%s", f.NodeId)
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
	// 心跳**不刷新** lan_addrs / lan_addr：agent 只在注册时上报候选内网地址，
	// 心跳报文里根本没有这两个字段（此前这里的分支永远走不到，是死代码）。
	// 网卡增减（插拔网线 / VPN 断开）靠节点重新注册同步，注册接口才是唯一入口。
	return DB.Model(&model.ServerNode{}).Where("node_id = ?", f.NodeId).Updates(updates).Error
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
		// 在线与否**只由心跳新鲜度决定**。探测失败不能判离线：
		// 节点与 api-server 同机时，探测公网地址会因缺少 NAT 回环而失败，
		// 之前因此把正常心跳的节点误标成离线。
		if nodes[i].LastSeen == 0 || now-nodes[i].LastSeen > offline {
			nodes[i].Status = 0
		} else {
			nodes[i].Status = 1
		}
	}
	// 列表里实时算一次分数（不落库），保证后台总能显示，
	// 不必等到下一次 Relays() 才写回；成本为 0 也是合法值。
	w := s.Weights()
	for i := range nodes {
		nodes[i].Score = s.Score(&nodes[i], w)
		// 候选内网地址：库里存逗号分隔串，这里转成数组给前端渲染（多网卡逐个展示）
		nodes[i].LanAddrsList = splitLanAddrs(nodes[i].LanAddrs)
		// 升级前的旧数据只有单值 lan_addr、没有 lan_addrs：用单值兜底成一条候选，
		// 否则「详情」里能看到地址、表格里却什么都不显示，两边对不上。
		if len(nodes[i].LanAddrsList) == 0 && strings.TrimSpace(nodes[i].LanAddr) != "" {
			nodes[i].LanAddrsList = []string{strings.TrimSpace(nodes[i].LanAddr)}
		}
	}
	return nodes
}

// RelayLanMap 返回「公网中继地址 → 候选内网地址列表」映射。
//
// 只统计已注册的 hbbr 节点（hbbs 不是中继）。客户端拿到映射后，选中某台公网中继时
// 可以再试它对应的内网地址：可达且明显更快就走内网，通告给对端的仍是公网地址。
//
// 过滤规则（客户端会照着逐个探测，脏数据等于白白多等一次超时）：
//   - 只算在线节点（LastSeen 新鲜），与 Relays() 的离线判定保持一致；
//   - 候选必须是合法 host:port，且不能等于该节点的公网地址（自映射没有意义）；
//   - 候选去重，保序。
func (s *NodeService) RelayLanMap() map[string][]string {
	var nodes []model.ServerNode
	if err := DB.Where("kind = ? AND disabled = 0", "hbbr").Find(&nodes).Error; err != nil {
		return map[string][]string{}
	}
	offline := s.OfflineSec()
	now := time.Now().Unix()
	out := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		addr := strings.TrimSpace(n.Addr)
		if addr == "" {
			continue
		}
		// 离线节点的内网地址大概率已经变了，下发只会让客户端等一次 800ms 超时。
		if n.LastSeen == 0 || now-n.LastSeen > offline {
			continue
		}
		lans := splitLanAddrs(n.LanAddrs)
		if len(lans) == 0 && strings.TrimSpace(n.LanAddr) != "" {
			lans = []string{strings.TrimSpace(n.LanAddr)}
		}
		seen := make(map[string]bool, len(lans))
		clean := make([]string, 0, len(lans))
		for _, a := range lans {
			a = strings.TrimSpace(a)
			if a == "" || a == addr || seen[a] || !ValidHostPort(a) {
				continue
			}
			seen[a] = true
			clean = append(clean, a)
		}
		if len(clean) == 0 {
			continue
		}
		out[addr] = clean
	}
	return out
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
	// 延迟：RttMs < 0 表示未知/探测失败，按最大成本 1.0 计。
	// 绝不能把"未知"当成 0ms——那会让探测不到的节点稳排第一。
	sRtt := 1.0
	if n.RttMs >= 0 {
		sRtt = normalize(float64(n.RttMs), 500)
	}
	// 人工权重：语义是"越希望用它越调大"，这里统一折算成**成本**：
	// 权重 100 → 成本 0；权重 0 → 成本 1。
	// 这样所有项都是"成本"，方向一致（总和越小越优先），
	// 不会再出现"负载项看小、人工项看大"的混乱。
	sWeightCost := 1 - normalize(float64(n.Weight), 100)

	score := w.Sessions*sSess +
		w.Bandwidth*sBw +
		w.CPU*sCpu +
		w.Mem*sMem +
		w.RTT*sRtt +
		w.Weight*sWeightCost
	return score
}

// Relays 返回按调度分数排序的可用中继列表（供 hbbs 拉取，或直接下发给客户端）。
//
// 内容 = 已注册在线且未下线的 hbbr（按分数排序）+ 静态中继兜底（去重）。
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
	// 先合并去重（调度结果 + 静态兜底），再统一按 limit 截断。
	// 顺序很关键：若先按 limit 截断调度结果，静态地址又恰好是"被截掉的
	// 在线节点"，去重逻辑会把它也丢掉，最终该地址彻底消失。
	seen := make(map[string]bool, len(list)+8)
	out := make([]map[string]interface{}, 0, len(list)+8)
	for _, it := range list {
		if it.node.Addr == "" || seen[it.node.Addr] {
			continue
		}
		seen[it.node.Addr] = true
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
		// 候选内网地址数组：hbbs 会把它们摊平成额外的中继候选下发给客户端，
		// 同内网的客户端实测更快就会走内网地址（hbbr 按 uuid 配对，不看接入地址）。
		if la := splitLanAddrs(it.node.LanAddrs); len(la) > 0 {
			item["lan_addrs"] = la
		}
		out = append(out, item)
	}
	// 追加静态中继兜底：保证未接入 agent 的中继不会被调度结果挤掉。
	// 调度只覆盖"已注册且在线"的节点，过渡期内中继数量可能少于静态配置。
	for _, addr := range s.StaticRelays() {
		if seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, map[string]interface{}{
			"addr":   addr,
			"score":  9999.0,
			"region": "",
			"static": true,
		})
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ValidHostPort 校验地址是否为合法的 host:port（节点地址 / 静态中继共用）。
func ValidHostPort(addr string) bool {
	addr = strings.TrimSpace(addr)
	host, port, err := net.SplitHostPort(addr)
	if err != nil || strings.TrimSpace(host) == "" || port == "" {
		return false
	}
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p > 65535 {
		return false
	}
	return true
}

// StaticRelays 读取静态中继兜底列表（逗号分隔）。
func (s *NodeService) StaticRelays() []string {
	v, ok := AllService.SettingService.Get(SettingKeyNodeStaticRelays)
	if !ok || v == "" {
		return nil
	}
	out := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	for _, a := range strings.Split(v, ",") {
		a = strings.TrimSpace(a)
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// SaveStaticRelays 保存静态中继兜底列表（去空白、去重、校验 host:port）。
func (s *NodeService) SaveStaticRelays(list []string) error {
	cleaned := make([]string, 0, len(list))
	seen := make(map[string]bool, len(list))
	for _, a := range list {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] {
			continue
		}
		if !ValidHostPort(a) {
			return fmt.Errorf("非法中继地址 %q（应为 host:port，如 1.2.3.4:21117）", a)
		}
		seen[a] = true
		cleaned = append(cleaned, a)
	}
	return AllService.SettingService.Set(SettingKeyNodeStaticRelays, strings.Join(cleaned, ","))
}

// NodeSecret 读取节点专用密钥（未配置返回空串）。
func (s *NodeService) NodeSecret() string {
	v, _ := AllService.SettingService.Get(SettingKeyNodeSecret)
	return strings.TrimSpace(v)
}

// NodeSecretPrev 读取上一把节点密钥（从未轮换过返回空串）。
func (s *NodeService) NodeSecretPrev() string {
	v, _ := AllService.SettingService.Get(SettingKeyNodeSecretPrev)
	return strings.TrimSpace(v)
}

// NodeSecretRotatedAt 最近一次密钥轮换时间（unix 秒，0 表示从未轮换/未知）。
func (s *NodeService) NodeSecretRotatedAt() int64 {
	n, ok := AllService.SettingService.GetInt(SettingKeyNodeSecretRotatedAt)
	if !ok || n <= 0 {
		return 0
	}
	return int64(n)
}

// NodeSecretGraceDeadline 旧密钥的宽限期截止时间（unix 秒）。
// 只有真的存在上一把密钥且轮换时间有效时才返回非 0。
func (s *NodeService) NodeSecretGraceDeadline() int64 {
	if s.NodeSecretPrev() == "" {
		return 0
	}
	rotatedAt := s.NodeSecretRotatedAt()
	if rotatedAt <= 0 {
		return 0
	}
	return rotatedAt + NodeSecretGraceSeconds
}

// NodeSecretPrevInGrace 上一把密钥当前是否仍在宽限期内。
func (s *NodeService) NodeSecretPrevInGrace() bool {
	deadline := s.NodeSecretGraceDeadline()
	return deadline > 0 && time.Now().Unix() <= deadline
}

// NodeSecretValid 校验请求携带的密钥。
//
// 接受：当前密钥；或轮换后宽限期内的上一把密钥（避免轮换瞬间节点全掉线）。
// 返回 (是否通过, 是否用的是上一把)。比较使用常量时间实现。
func (s *NodeService) NodeSecretValid(provided string) (bool, bool) {
	provided = strings.TrimSpace(provided)
	if provided == "" {
		return false, false
	}
	current := s.NodeSecret()
	if current != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(current)) == 1 {
		return true, false
	}
	if s.NodeSecretPrevInGrace() {
		prev := s.NodeSecretPrev()
		if prev != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(prev)) == 1 {
			return true, true
		}
	}
	return false, false
}

// RotateNodeSecret 轮换节点专用密钥。
//
// 顺序：先保存上一把（宽限期用），再写新值，最后记录轮换时间。
// 这样即使进程在中间崩溃，也不会出现"新值已生效但旧值没保留"的掉线窗口。
func (s *NodeService) RotateNodeSecret(secret string) (int64, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return 0, fmt.Errorf("密钥不能为空")
	}
	prev := s.NodeSecret()
	if prev != "" && subtle.ConstantTimeCompare([]byte(prev), []byte(secret)) == 1 {
		// 值没变：不轮换，也不刷新时间戳（否则宽限期会被无限续命）
		return s.NodeSecretRotatedAt(), nil
	}
	now := time.Now().Unix()
	// 1) 保留上一把（首次配置时 prev 为空，直接跳过）
	if err := AllService.SettingService.Set(SettingKeyNodeSecretPrev, prev); err != nil {
		return 0, err
	}
	// 2) 写入新值
	if err := AllService.SettingService.Set(SettingKeyNodeSecret, secret); err != nil {
		return 0, err
	}
	// 3) 记录轮换时间（宽限期计算基准）
	if err := AllService.SettingService.Set(SettingKeyNodeSecretRotatedAt, strconv.FormatInt(now, 10)); err != nil {
		return 0, err
	}
	return now, nil
}

// TouchRtt 健康检查时回填 api-server 到该节点的 TCP 建连耗时（-1 = 未知/失败）。
func (s *NodeService) TouchRtt(id uint, rttMs int) {
	res := DB.Model(&model.ServerNode{}).Where("id = ?", id).Update("rtt_ms", rttMs)
	// 诊断：id 为 0 或影响行数为 0 时 rtt 永远写不进去，必须显式暴露出来，
	// 否则只看到"探测成功"却查不到数据，极难定位。
	if res.Error != nil {
		global.Logger.Warnf("[Node] 写入 rtt 失败 id=%d rtt=%d: %v", id, rttMs, res.Error)
	} else if res.RowsAffected == 0 {
		global.Logger.Warnf("[Node] 写入 rtt 未命中 id=%d rtt=%d（可能 Id 为空）", id, rttMs)
	}
}

// probeTimeout 单个候选地址的探测超时。
//
// 候选可能有好几个（多网卡），太长的超时会让一轮健康检查拖过 30 秒的 tick 间隔。
const probeTimeout = 800 * time.Millisecond

// probeConcurrency 一轮健康检查中同时探测的**节点**数上限。
//
// 不能无限并发：节点多时会同时发起大量带 800ms 超时的 TCP 连接，把本地
// fd / 连接表打满；4 个足够在 30s tick 内探完几十台节点（每台最坏 ~800ms）。
const probeConcurrency = 4

// probeAllTimeout 整轮健康检查的截止时间，到点后不再启动新的节点探测。
//
// 给 20s 是留出 10s 余量：tick 间隔 30s，探测必须明显短于它，
// 否则下一轮会被上一轮拖住，表现为"健康检查越跑越慢/干脆不跑"。
const probeAllTimeout = 20 * time.Second

// probeCandidate 探测单个候选地址，返回（毫秒, 是否可达）。
//
// 用 Dialer.DialContext 而不是 net.DialTimeout：整轮超时（ctx）能取消在途连接，
// 单个地址仍由 Dialer.Timeout 兜底。
func probeCandidate(ctx context.Context, target string) (int, bool) {
	begin := time.Now()
	d := net.Dialer{Timeout: probeTimeout}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		global.Logger.Debugf("[Node] 探测 %s 失败: %v", target, err)
		return 0, false
	}
	_ = conn.Close()
	return int(time.Since(begin).Milliseconds()), true
}

// buildProbeCandidates 汇总该节点的全部可探测地址：lan_addrs 全体 → lan_addr → 公网 addr。
//
// 顺序有意义：并发探测后若多个候选延迟相同，优先靠前的（内网地址在内网场景下
// 才是真正想选的那个）。
func buildProbeCandidates(n *model.ServerNode) []string {
	candidates := make([]string, 0, maxLanAddrs+2)
	seen := make(map[string]bool, maxLanAddrs+2)
	add := func(a string) {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] {
			return
		}
		seen[a] = true
		candidates = append(candidates, a)
	}
	for _, a := range splitLanAddrs(n.LanAddrs) {
		add(a)
	}
	add(n.LanAddr) // 兼容只上报单值的旧节点
	add(n.Addr)    // 最后兜底公网地址
	return candidates
}

// Probe 探测节点 TCP 可达性并回填 RTT（单节点入口，内部并发探测所有候选）。
func (s *NodeService) Probe(n *model.ServerNode) {
	s.probeWithContext(context.Background(), n)
}

// probeWithContext 并发探测全部候选，取延迟最低的成功项写 rtt_ms。
//
// 节点会自动上报**全部**候选内网地址（多网卡：业务网 / VPN / Docker 网桥），
// 这里并发探测、取延迟最低的那个写 rtt_ms —— 由 api-server 侧做选择，
// 而不是让节点猜哪个地址是对的：
//
//   - 容器里上报的可能是容器内网 IP，api-server 探不到 → 跳过，继续看别的
//   - 同机部署时公网地址没有 NAT 回环 → 探不到，内网地址会命中
//   - 全部候选都不通才记 -1（未知，评分按最大成本），不能记 0
//
// 必须并发：串行时单节点最坏耗时 = 候选数 × probeTimeout（10 个候选就是 8s），
// 4~5 台节点就能把 30s 的 tick 拖穿，健康检查滞后到形同停摆。
func (s *NodeService) probeWithContext(ctx context.Context, n *model.ServerNode) {
	candidates := buildProbeCandidates(n)
	if len(candidates) == 0 {
		DB.Model(&model.ServerNode{}).Where("id = ?", n.Id).Update("rtt_ms", -1)
		return
	}
	// 每个 goroutine 只写自己那一格，天然没有数据竞争；用切片而不是 channel
	// 可以保留候选顺序（延迟相同时择优更稳定）。
	type result struct {
		ms int
		ok bool
	}
	results := make([]result, len(candidates))
	var wg sync.WaitGroup
	for i := range candidates {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ms, ok := probeCandidate(ctx, candidates[idx])
			results[idx] = result{ms: ms, ok: ok}
		}(i)
	}
	wg.Wait()

	bestMs, bestAddr, okCount := -1, "", 0
	for i, r := range results {
		if !r.ok {
			continue
		}
		okCount++
		if bestMs < 0 || r.ms < bestMs {
			bestMs, bestAddr = r.ms, candidates[i]
		}
	}
	if bestMs < 0 {
		global.Logger.Warnf("[Node] 探测 %s 全部候选失败（共 %d 个），rtt 记为未知", n.NodeId, len(candidates))
		DB.Model(&model.ServerNode{}).Where("id = ?", n.Id).Update("rtt_ms", -1)
		return
	}
	global.Logger.Infof("[Node] 探测选中 %s %dms（候选 %d 个，可达 %d 个）", bestAddr, bestMs, len(candidates), okCount)
	s.TouchRtt(n.Id, bestMs)
}

// ProbeAll 并发探测一批节点（信号量限流 + 整轮截止时间），供定时健康检查与
// 后台「立即刷新」共用。
//
// 入参是**值切片**：每个 goroutine 各自持有一份 model.ServerNode 拷贝，
// 绝不把同一个 *model.ServerNode 交给多个 goroutine —— 写库一律按 id，
// 不依赖指针身份，避免并发复用同一指针带来的数据竞争。
func (s *NodeService) ProbeAll(nodes []model.ServerNode) {
	if len(nodes) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeAllTimeout)
	defer cancel()
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for i := range nodes {
		select {
		case <-ctx.Done():
			// 整轮已到截止时间：不再启动新探测，等在途的收尾后返回。
			global.Logger.Warnf("[Node] 健康检查已达 %v 截止时间，跳过剩余 %d 个节点",
				probeAllTimeout, len(nodes)-i)
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(node model.ServerNode) {
			defer wg.Done()
			defer func() { <-sem }()
			s.probeWithContext(ctx, &node)
		}(nodes[i])
	}
	wg.Wait()
}

// StartHealthCheck 定时健康检查：探测可达性并回填 RTT（在线状态由心跳判定）。
//
// 并发只发生在 tick 内部（ProbeAll），不新增启动期 goroutine ——
// 项目约定所有后台 goroutine 必须写在 http.ApiInit() 之前，由 apimain 统一启动。
func (s *NodeService) StartHealthCheck() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		nodes := s.List("")
		s.ProbeAll(nodes)
	}
}

// PeerRegion 读取设备已设置的地域代码（无则空串）。
func (s *NodeService) PeerRegion(peerId string) string {
	if peerId == "" {
		return ""
	}
	var peer model.Peer
	if err := DB.Where("id = ?", peerId).First(&peer).Error; err != nil {
		return ""
	}
	return peer.Region
}

// SetPeerRegion 设置设备地域代码（后台指定；客户端自报也走这里，但优先级更高）。
func (s *NodeService) SetPeerRegion(peerId, region string) error {
	if peerId == "" {
		return fmt.Errorf("peer_id 不能为空")
	}
	if !ValidProvinceCode(region) {
		return fmt.Errorf("非法地域代码：%s", region)
	}
	return DB.Model(&model.Peer{}).Where("id = ?", peerId).Update("region", region).Error
}

// GetConfig 读取节点配置（JSON 文本）与其版本号。
//
// 版本号用 ConfigVersion（仅 SaveConfig 时 +1），不能用行上的 UpdatedAt：
// 心跳/探测/写评分都会刷新 UpdatedAt，版本号会每 15 秒变一次。
func (s *NodeService) GetConfig(nodeId string) (string, int64, error) {
	var node model.ServerNode
	if err := DB.Where("node_id = ?", nodeId).First(&node).Error; err != nil {
		return "", 0, fmt.Errorf("节点不存在：%s", nodeId)
	}
	return node.Config, node.ConfigVersion, nil
}

// SaveConfig 保存节点配置（管理员下发）。返回新版本号。
//
// 配置约定（与节点 agent 侧一致）：
//   - relay_servers: []string，元素为 host:port
//   - limit_speed_mbps / single_bandwidth_mbps / total_bandwidth_mbps: 整数，Mbit/s
//   - downgrade_threshold: 浮点 0~1（比例，不是百分数）
//   - downgrade_start_check_sec: 整数，秒
func (s *NodeService) SaveConfig(id uint, cfg string) (int64, error) {
	if strings.TrimSpace(cfg) == "" {
		cfg = "{}"
	}
	if len(cfg) > maxNodeConfigBytes {
		return 0, fmt.Errorf("配置过大：%d 字节（上限 %d 字节）", len(cfg), maxNodeConfigBytes)
	}
	var probe map[string]interface{}
	if err := json.Unmarshal([]byte(cfg), &probe); err != nil {
		return 0, fmt.Errorf("配置不是合法 JSON：%v", err)
	}
	if err := validateNodeConfig(probe); err != nil {
		return 0, err
	}
	res := DB.Model(&model.ServerNode{}).Where("id = ?", id).Updates(map[string]interface{}{
		"config":         cfg,
		"config_version": gorm.Expr("config_version + 1"),
	})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, fmt.Errorf("节点不存在：id=%d", id)
	}
	var node model.ServerNode
	if err := DB.Where("id = ?", id).First(&node).Error; err != nil {
		return 0, err
	}
	return node.ConfigVersion, nil
}

// validateNodeConfig 校验节点配置里已知键的量纲，避免后台填错单位后静默生效。
// 未知键不校验（便于节点侧扩展）。
func validateNodeConfig(cfg map[string]interface{}) error {
	if v, ok := cfg["relay_servers"]; ok && v != nil {
		arr, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("relay_servers 必须是字符串数组")
		}
		for _, it := range arr {
			str, ok := it.(string)
			if !ok {
				return fmt.Errorf("relay_servers 元素必须是字符串")
			}
			if str == "" {
				continue
			}
			if !ValidHostPort(str) {
				return fmt.Errorf("relay_servers 元素 %q 非法（应为 host:port）", str)
			}
		}
	}
	intKeys := []string{"limit_speed_mbps", "single_bandwidth_mbps", "total_bandwidth_mbps", "downgrade_start_check_sec"}
	for _, k := range intKeys {
		v, ok := cfg[k]
		if !ok || v == nil {
			continue
		}
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("%s 必须是数字", k)
		}
		if f < 0 || f > 1e9 || f != float64(int64(f)) {
			return fmt.Errorf("%s 必须是非负整数", k)
		}
	}
	if v, ok := cfg["downgrade_threshold"]; ok && v != nil {
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("downgrade_threshold 必须是数字")
		}
		if f < 0 || f > 1 {
			return fmt.Errorf("downgrade_threshold 必须在 0~1 之间（比例，不是百分数）")
		}
	}
	return nil
}

// MarkOverrides 记录管理员在后台接管的字段（追加合并，去重）。
//
// 典型场景：节点上报的 addr 是自动探测结果，管理员发现不对在后台改掉，
// 若不记录接管，下一次注册又会被节点上报值冲掉。
func (s *NodeService) MarkOverrides(id uint, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	var node model.ServerNode
	if err := DB.Where("id = ?", id).First(&node).Error; err != nil {
		return err
	}
	set := overrideSet(parseOverrideList(node.Overrides))
	for _, k := range fields {
		if k != "" {
			set[k] = true
		}
	}
	list := make([]string, 0, len(set))
	for k := range set {
		list = append(list, k)
	}
	sort.Strings(list)
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return DB.Model(&model.ServerNode{}).Where("id = ?", id).Update("overrides", string(b)).Error
}

// UnlockOverrides 解除平台接管：fields 为空表示解除全部，否则只解除列出的字段。
//
// 没有这个入口时 Overrides 是"单行道"：管理员一旦接管过某字段，节点自身
// 上报的新值（例如换机后的内网地址）永远同步不回来，只能手工改库。
func (s *NodeService) UnlockOverrides(id uint, fields []string) error {
	var node model.ServerNode
	if err := DB.Where("id = ?", id).First(&node).Error; err != nil {
		return err
	}
	if len(fields) == 0 {
		return DB.Model(&model.ServerNode{}).Where("id = ?", id).Update("overrides", "").Error
	}
	drop := make(map[string]bool, len(fields))
	for _, k := range fields {
		if k != "" {
			drop[k] = true
		}
	}
	kept := make([]string, 0, 4)
	for _, k := range parseOverrideList(node.Overrides) {
		if !drop[k] {
			kept = append(kept, k)
		}
	}
	sort.Strings(kept)
	b, err := json.Marshal(kept)
	if err != nil {
		return err
	}
	return DB.Model(&model.ServerNode{}).Where("id = ?", id).Update("overrides", string(b)).Error
}
