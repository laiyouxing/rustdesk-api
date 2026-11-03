package model

const (
	AuditActionNew   = "new"
	AuditActionClose = "close"
)

type AuditConn struct {
	IdModel
	Action    string `json:"action" gorm:"default:'';not null;"`
	ConnId    int64  `json:"conn_id" gorm:"default:0;not null;index"`
	PeerId    string `json:"peer_id" gorm:"default:'';not null;index"`
	FromPeer  string `json:"from_peer" gorm:"default:'';not null;"`
	FromName  string `json:"from_name" gorm:"default:'';not null;"`
	Ip        string `json:"ip" gorm:"default:'';not null;"`
	SessionId string `json:"session_id" gorm:"default:'';not null;"`
	Type      int    `json:"type" gorm:"default:0;not null;"`
	Uuid      string `json:"uuid" gorm:"default:'';not null;"`
	CloseTime int64  `json:"close_time" gorm:"default:0;not null;"`
	// AttemptCount 这次操作里实际发起了几次连接尝试。
	// 网络抖动 / NAT 打洞失败会让客户端对同一次点击重试多次，服务端每次都会收到
	// 独立的 new 审计；服务端会把它们合并成一条记录，用这个字段记下合并了几次。
	AttemptCount int `json:"attempt_count" gorm:"default:1;not null;"`
	// OpId 控制端「一次操作」的标识（1.7.32+ 客户端才有，老客户端为 0）。
	// 同一次点击连接引发的所有重试都带同一个 op_id，服务端按它精确合并记录；
	// 为 0 时回退到「同 from_peer + 时间窗」的推断（见 service.ConnMergeWindow）。
	OpId uint64 `json:"op_id" gorm:"default:0;not null;"`
	// Version 被控端（peer_id 那台设备）的客户端版本。
	Version string `json:"version" gorm:"default:'';not null;"`
	// FromVersion 控制端（from_peer 那台设备）的客户端版本。
	FromVersion string `json:"from_version" gorm:"default:'';not null;"`
	// PeerHostname / PeerAlias 被控端的主机名与别名（写入时快照，设备改名后历史不变）。
	PeerHostname string `json:"peer_hostname" gorm:"default:'';not null;"`
	PeerAlias    string `json:"peer_alias" gorm:"default:'';not null;"`
	// FromHostname / FromAlias 控制端的主机名与别名。
	FromHostname string `json:"from_hostname" gorm:"default:'';not null;"`
	FromAlias    string `json:"from_alias" gorm:"default:'';not null;"`
	TimeModel
}

type AuditConnList struct {
	AuditConns []*AuditConn `json:"list"`
	Pagination
}

type AuditFile struct {
	IdModel
	FromPeer string `json:"from_peer" gorm:"default:'';not null;index"`
	Info     string `json:"info" gorm:"default:'';not null;"`
	IsFile   bool   `json:"is_file" gorm:"default:0;not null;"`
	Path     string `json:"path" gorm:"default:'';not null;"`
	PeerId   string `json:"peer_id" gorm:"default:'';not null;index"`
	Type     int    `json:"type" gorm:"default:0;not null;"`
	Uuid     string `json:"uuid" gorm:"default:'';not null;"`
	Ip       string `json:"ip" gorm:"default:'';not null;"`
	Num      int    `json:"num" gorm:"default:0;not null;"`
	FromName string `json:"from_name" gorm:"default:'';not null;"`
	// Version 被控端（peer_id 那台设备）的客户端版本。
	Version string `json:"version" gorm:"default:'';not null;"`
	// FromVersion 控制端（from_peer 那台设备）的客户端版本。
	FromVersion string `json:"from_version" gorm:"default:'';not null;"`
	// PeerHostname / PeerAlias 被控端的主机名与别名（写入时快照）。
	PeerHostname string `json:"peer_hostname" gorm:"default:'';not null;"`
	PeerAlias    string `json:"peer_alias" gorm:"default:'';not null;"`
	// FromHostname / FromAlias 控制端的主机名与别名。
	FromHostname string `json:"from_hostname" gorm:"default:'';not null;"`
	FromAlias    string `json:"from_alias" gorm:"default:'';not null;"`
	TimeModel
}

type AuditFileList struct {
	AuditFiles []*AuditFile `json:"list"`
	Pagination
}
