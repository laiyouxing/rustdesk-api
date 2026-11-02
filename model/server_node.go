package model

// ServerNode 集群节点（hbbs / hbbr）。
//
// 背景：原先中继列表写死在 hbbs 的 `-r` 参数里，加/减一台中继要改配置重启。
// 这里把节点登记到 api-server，由 api-server 统一做健康检查与负载评分，
// hbbs 定期拉取"排好序的中继列表"，客户端据此择优。
//
// 一个物理机可以同时跑 hbbs 和 hbbr，所以用 NodeId（kind:addr）区分，
// 而不是用 IP。
type ServerNode struct {
	Id uint `json:"id" gorm:"primaryKey"`

	// NodeId 节点唯一标识，约定格式 `{kind}@{addr}`，例如 `hbbr@120.236.237.39:21117`
	NodeId string `json:"node_id" gorm:"size:128;uniqueIndex;not null"`
	// Kind hbbs | hbbr
	Kind string `json:"kind" gorm:"size:16;not null;index"`
	Name string `json:"name" gorm:"size:100"`
	// Addr 对外地址（公网）host:port
	Addr string `json:"addr" gorm:"size:200;not null"`
	// LanAddr 内网地址（可选），供同内网客户端直连，避免绕公网。
	// 多网卡时取 LanAddrs 的第一个。
	LanAddr string `json:"lan_addr" gorm:"size:200"`
	// LanAddrs 节点自动上报的全部候选内网地址（逗号分隔，最多 8 个）。
	//
	// 一台机器常有业务网 / VPN / Docker 网桥多张网卡，api-server 探测时逐个尝试、
	// 取延迟最低的那个写 rtt_ms，所以要把候选全存下来，而不是只留一个。
	LanAddrs string `json:"lan_addrs" gorm:"size:512"`
	// Region 地域标识（省级 code，如 GD/GX），用于就近调度
	Region string `json:"region" gorm:"size:64;index"`
	// Version 节点上报的版本号
	Version string `json:"version" gorm:"size:64"`
	// Weight 人工权重：越大越优先（评分时折算为负向）
	Weight int `json:"weight" gorm:"default:100"`
	// MaxSessions 会话上限，0 表示不限（用于负载归一化）
	MaxSessions int `json:"max_sessions" gorm:"default:0"`
	// Disabled 管理员手动下线：1=不再下发
	Disabled int `json:"disabled" gorm:"default:0"`
	// Status 运行状态：1=在线，0=离线（由心跳超时判定）
	Status int `json:"status" gorm:"default:1;index"`

	// ===== 心跳上报的实时指标 =====
	Peers     int64 `json:"peers"`
	Sessions  int64 `json:"sessions"`
	BwIn      int64 `json:"bw_in"`
	BwOut     int64 `json:"bw_out"`
	CpuPct    int   `json:"cpu_pct"`
	MemPct    int   `json:"mem_pct"`
	UptimeSec int64 `json:"uptime_sec"`
	// RttMs api-server 侧对节点的探测延迟（由健康检查回填）。
	//
	// 语义：-1 = 未知/探测失败，>=0 = 真实毫秒。
	// 必须区分「未知」与「0ms」：评分时未知按最大成本计算，
	// 否则探测不到的节点会被当成 0ms 排到最前。
	RttMs int `json:"rtt_ms"`

	// Score 最近一次计算的负载评分（越小越优先），便于前端直接展示
	Score float64 `json:"score"`
	// LanAddrsList 候选内网地址数组，**仅用于 JSON 输出**（列表接口填充），
	// 库里存的是逗号分隔串 LanAddrs，前端要按数组渲染。
	LanAddrsList []string `json:"lan_addrs_list" gorm:"-"`
	// LastSeen 最后一次心跳时间（unix 秒）
	LastSeen int64 `json:"last_seen" gorm:"index"`

	// Config 由 api-server 下发的节点配置（JSON 文本）。
	//
	// 设计：节点自身只保留"启动参数"，其余可调项全部集中在这里，
	// 节点 agent 定期拉取并热应用（中继列表 / 限速等），
	// 无法热改的（key/端口/mask）只做展示，需重启生效。
	Config string `json:"config" gorm:"type:text"`

	// ConfigVersion 配置版本号：仅在 SaveConfig 时 +1。
	//
	// 注意：不能用行上的 UpdatedAt 当配置版本——心跳/探测/写评分都会刷新
	// UpdatedAt，导致版本号每 15 秒变一次，节点无法据此判断配置是否变化。
	ConfigVersion int64 `json:"config_version" gorm:"default:0"`

	// Overrides 平台已接管的字段名（JSON 数组，如 ["addr","name"]）。
	//
	// 冲突原则：**平台下发优先**。节点注册上报的 addr/lan_addr/name/
	// max_sessions/region 若已被管理员在后台改过，就不再被节点覆盖，
	// 否则后台改完下一轮心跳注册又会被冲回去。
	Overrides string `json:"overrides" gorm:"type:text"`

	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ServerNode) TableName() string {
	return "server_nodes"
}
