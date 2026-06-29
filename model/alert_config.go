package model

type AlertConfig struct {
	RowId      uint   `json:"row_id" gorm:"primaryKey"`
	UserId     uint   `json:"user_id" gorm:"default:0;not null;index"`           // who created this config
	Channel    string `json:"channel" gorm:"size:32;not null;default:''"`        // station/wecom/dingtalk/smtp
	Name       string `json:"name" gorm:"size:100;not null;default:''"`          // display name
	WebhookUrl string `json:"webhook_url" gorm:"size:500;not null;default:''"`   // webhook URL for wecom/dingtalk
	SmtpHost   string `json:"smtp_host" gorm:"size:200;not null;default:''"`     // SMTP host
	SmtpPort   int    `json:"smtp_port" gorm:"default:0"`                        // SMTP port
	SmtpUser   string `json:"smtp_user" gorm:"size:200;not null;default:''"`     // SMTP username
	SmtpPass   string `json:"smtp_pass" gorm:"size:200;not null;default:''"`     // SMTP password
	SmtpTo     string `json:"smtp_to" gorm:"size:500;not null;default:''"`       // recipient email(s), comma separated
	OfflineMin int    `json:"offline_min" gorm:"default:5"`                      // offline threshold in minutes
	Enabled    int    `json:"enabled" gorm:"default:1"`                          // 1=enabled 2=disabled
	MonitorAll int    `json:"monitor_all" gorm:"default:1"`                      // 1=monitor all peers, 2=monitor selected targets only
}

func (AlertConfig) TableName() string {
	return "alert_configs"
}
