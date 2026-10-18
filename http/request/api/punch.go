package api

// PunchAddrItem 单个打洞地址
type PunchAddrItem struct {
	Addr     string `json:"addr"`       // "ip:port"
	AddrType string `json:"addr_type"`  // udp | udp6 | tcp
	Delta    int32  `json:"delta"`      // 对称 NAT 端口增量（0 表示锥形）
	SyncAtMs int64  `json:"sync_at_ms"` // NTP 对齐的双方约定起始时刻
}

// PunchAddrForm 客户端上报自己的打洞地址（STUN 探测结果）
// Target 可选：控制端连接时带上被控端 id，被控端据此发现"有人要连我"
type PunchAddrForm struct {
	PeerId string          `json:"peer_id"`
	Target string          `json:"target"`
	Addrs  []PunchAddrItem `json:"addrs"`
}

// PunchAddrQuery 拉取地址的查询参数
type PunchAddrQuery struct {
	PeerId string `json:"peer_id" form:"peer_id"`
}
