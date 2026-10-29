package model

// PunchDirectOk 直连会话合规上报记录。
//
// 客户端在直连（IPv4/IPv6）建立成功后上报本机网络标识；服务端据此确认
// "这条直连会话已登记"。等宽限期内没收到上报的会话会被强制断开，
// 因此这张表同时是**执法依据**与**合规审计留痕**，后台审计页可查。
//
// 安全/隐私约束（重要，勿改）：
//   - MacHashes 存的是客户端算好的 sha256(服务器密钥||MAC) 哈希，
//     **不接收、不存储原始 MAC**；
//   - 只有元数据（谁连谁、何时、地址、哈希），**没有会话内容**——
//     连接是端到端加密的，服务端不解。
type PunchDirectOk struct {
	IdModel
	PeerId     string `json:"peer_id" gorm:"default:'';not null;index"`
	Target     string `json:"target" gorm:"default:'';not null;index"`
	// direct_v4 | direct_v6 | relay（relay 表示打洞失败回退中继，属正常路径）
	ConnType   string `json:"conn_type" gorm:"default:'';not null;index"`
	PublicAddr string `json:"public_addr" gorm:"default:'';not null"`
	// 逗号分隔。内网地址可能多个；MAC 哈希同理。
	LanAddrs  string `json:"lan_addrs" gorm:"default:'';not null"`
	MacHashes string `json:"mac_hashes" gorm:"default:'';not null"`
	TimeModel
}

type PunchDirectOkList struct {
	PunchDirectOks []*PunchDirectOk `json:"list"`
	Pagination
}
