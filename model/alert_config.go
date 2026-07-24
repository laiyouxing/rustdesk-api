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
	Recipients             string `json:"recipients" gorm:"size:500; not null;default:''"`  // 接收人（SMTP 收件人邮箱，逗号分隔）；由发送配置决定
	LastNotifiedAt         int64  `json:"last_notified_at" gorm:"default:0"`
	NotifiedPeers          string `json:"notified_peers" gorm:"type:text"` // JSON map（含每设备通知次数、权重等）
	ConsecutiveTriggerDays int    `json:"consecutive_trigger_days" gorm:"default:0"` // 连续触发离线告警的天数
	LastTriggerDay         int    `json:"last_trigger_day" gorm:"default:0"`          // 最近一次触发告警的日期（YYYYMMDD），用于连续天数计算
	CreatedAt              int64  `json:"created_at" gorm:"autoCreateTime"`            // 告警创建时间戳，用于判断离线事件是否发生在创建之前
}

func (AlertConfig) TableName() string {
	return "alert_configs"
}
