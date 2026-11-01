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
	// LanAddr 内网地址（可选），供同内网客户端直连，避免绕公网
	LanAddr string `json:"lan_addr" gorm:"size:200"`
	// Region 地域标识（如 gz/bj/hc），用于就近调度
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
	// RttMs api-server 侧对节点的探测延迟（由健康检查回填）
	RttMs int `json:"rtt_ms"`

	// Score 最近一次计算的负载评分（越小越优先），便于前端直接展示
	Score float64 `json:"score"`
	// LastSeen 最后一次心跳时间（unix 秒）
	LastSeen int64 `json:"last_seen" gorm:"index"`

	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ServerNode) TableName() string {
	return "server_nodes"
}
