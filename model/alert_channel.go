package model

type AlertChannel struct {
	RowId      uint   `json:"row_id" gorm:"primaryKey"`
	UserId     uint   `json:"user_id" gorm:"default:0;not null;index"`
	Channel    string `json:"channel" gorm:"size:32;not null;default:''"`    // station/wecom/dingtalk/smtp
	Name       string `json:"name" gorm:"size:100;not null;default:''"`
	WebhookUrl string `json:"webhook_url" gorm:"size:500;not null;default:''"`
	SmtpHost   string `json:"smtp_host" gorm:"size:200;not null;default:''"`
	SmtpPort   int    `json:"smtp_port" gorm:"default:0"`
	SmtpUser   string `json:"smtp_user" gorm:"size:200;not null;default:''"`
	SmtpPass   string `json:"smtp_pass" gorm:"size:200;not null;default:''"`
	// 注意：接收人（收件人邮箱）不再属于通道，已迁移到 AlertConfig.Recipients，
	// 由“发送配置/告警规则”决定，避免同一通道被多个规则共用时收件人错乱。
	TimeModel
}

func (AlertChannel) TableName() string {
	return "alert_channels"
}
