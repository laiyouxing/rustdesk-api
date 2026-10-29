package api

// PunchAddrItem 单个打洞地址。
// Kind 标注地址来源，对端据此排优先级：
//
//	last_success  上一会话成功用过的映射（重连时 NAT 映射通常仍在，命中率最高）
//	stun_primary  主 STUN 服务器看到的映射
//	stun_alt      备 STUN 服务器看到的映射（对称 NAT 下与主不同）
//	upnp          UPnP 外部映射端口
//	lan           本机内网网卡地址（双方同网段时可用）
//	v6            公网 IPv6 地址（双栈互通）
//	tcp           TCP 同步打开监听端口（UDP 全封时的 fallback）
//
// Priority 为上报方建议的优先级（越大越先试）。
type PunchAddrItem struct {
	Addr     string `json:"addr"`       // "ip:port"
	AddrType string `json:"addr_type"`  // udp | udp6 | tcp
	Kind     string `json:"kind"`       // 地址来源（见上）
	Priority int32  `json:"priority"`   // 建议优先级，越大越先试
	Delta    int32  `json:"delta"`      // 对称 NAT 端口增量（相对主映射，0 = 锥形）
	SyncAtMs int64  `json:"sync_at_ms"` // NTP 对齐的双方约定起始时刻
}

// PunchMeta 设备网络环境（对端据此选择打洞策略）
type PunchMeta struct {
	NatType  string `json:"nat_type"`  // cone | symmetric | unknown
	Upnp     bool   `json:"upnp"`      // 是否有 UPnP 外部映射
	ViaProxy bool   `json:"via_proxy"` // 是否经 SOCKS/WS 代理（打洞基本无意义）
}

// PunchAddrForm 客户端上报自己的打洞地址（STUN 探测结果）
// Target 可选：控制端连接时带上被控端 id，被控端据此发现"有人要连我"
type PunchAddrForm struct {
	PeerId string          `json:"peer_id"`
	Target string          `json:"target"`
	Meta   PunchMeta       `json:"meta"`
	Addrs  []PunchAddrItem `json:"addrs"`
}

// PunchAddrQuery 拉取地址的查询参数
type PunchAddrQuery struct {
	PeerId string `json:"peer_id" form:"peer_id"`
}

// PunchDirectOkForm 直连建立成功后的合规上报。
//
// 客户端在直连（v4/v6）握手成功后立即上报；服务端若在宽限期内未收到两端
// 上报，会通过心跳下发 disconnect 强制断开该会话。
// 注意 MacHashes 是**哈希**，客户端不上报原始 MAC。
type PunchDirectOkForm struct {
	PeerId     string   `json:"peer_id"`
	Target     string   `json:"target"`
	ConnType   string   `json:"conn_type"`   // direct_v4 | direct_v6
	PublicAddr string   `json:"public_addr"` // 本机公网地址
	LanAddrs   []string `json:"lan_addrs"`   // 本机内网地址
	MacHashes  []string `json:"mac_hashes"`  // 本机 MAC 哈希（不含原始值）
}
