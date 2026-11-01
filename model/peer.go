package model

type Peer struct {
	RowId          uint   `json:"row_id" gorm:"primaryKey;"`
	Id             string `json:"id"  gorm:"default:'';not null;index"`
	Cpu            string `json:"cpu"  gorm:"default:'';not null;"`
	Hostname       string `json:"hostname"  gorm:"default:'';not null;"`
	Memory         string `json:"memory"  gorm:"default:'';not null;"`
	Os             string `json:"os"  gorm:"default:'';not null;"`
	Username       string `json:"username"  gorm:"default:'';not null;"`
	Uuid           string `json:"uuid"  gorm:"default:'';not null;index"`
	Version        string `json:"version"  gorm:"default:'';not null;"`
	UserId         uint   `json:"user_id"  gorm:"default:0;not null;index"`
	User           *User  `json:"user,omitempty"`
	LastOnlineTime int64  `json:"last_online_time"  gorm:"default:0;not null;"`
	LastOnlineIp   string `json:"last_online_ip"  gorm:"default:'';not null;"`
	GroupId        uint   `json:"group_id"  gorm:"default:0;not null;index"`
	Alias          string `json:"alias" gorm:"default:'';not null;index"`
	// Region 设备所属省级地域代码（如 GD/GX/AH）。
	//
	// 中继就近调度用：客户端可自行设置（优先级最高），后台也可指定。
	// 为空表示未设置，此时按纯延迟择优，行为与之前一致。
	Region string `json:"region" gorm:"default:'';not null;index"`
	TimeModel
}

type PeerList struct {
	Peers []*Peer `json:"list"`
	Pagination
}
