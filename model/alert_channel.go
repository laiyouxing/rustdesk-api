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
	SmtpTo     string `json:"smtp_to" gorm:"size:500;not null;default:''"`
	TimeModel
}

func (AlertChannel) TableName() string {
	return "alert_channels"
}
