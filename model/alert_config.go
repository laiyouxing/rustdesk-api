package model

type AlertConfig struct {
	RowId          uint   `json:"row_id" gorm:"primaryKey"`
	UserId         uint   `json:"user_id" gorm:"default:0;not null;index"`
	ChannelId      uint   `json:"channel_id" gorm:"default:0;not null;index"` // FK -> AlertChannel.row_id
	Channel        string `json:"channel" gorm:"size:32;not null;default:''"` // 冗余，方便查询
	Name           string `json:"name" gorm:"size:100;not null;default:''"`   // 规则名称
	OfflineMin     int    `json:"offline_min" gorm:"default:5"`
	Enabled        int    `json:"enabled" gorm:"default:1"`
	MonitorAll     int    `json:"monitor_all" gorm:"default:1"`
	LastNotifiedAt int64  `json:"last_notified_at" gorm:"default:0"`
	NotifiedPeers  string `json:"notified_peers" gorm:"type:text"` // JSON map
}

func (AlertConfig) TableName() string {
	return "alert_configs"
}
