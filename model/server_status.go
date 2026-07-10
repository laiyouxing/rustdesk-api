package model

import "time"

// ServerStatusMonitor 自定义服务器探测条目（用户在页面自行填写地址，可创建多个）
type ServerStatusMonitor struct {
	RowId     uint      `json:"row_id" gorm:"primaryKey"`
	UserId    uint      `json:"user_id" gorm:"default:0;not null;index"`        // 创建者；多用户隔离
	Name      string    `json:"name" gorm:"size:128;not null;default:''"`       // 显示名
	Host      string    `json:"host" gorm:"size:255;not null;default:''"`       // 主机/IP
	Port      int       `json:"port" gorm:"default:0"`                          // 端口；0 表示只探测主机连通性
	Protocol  string    `json:"protocol" gorm:"size:16;not null;default:'tcp'"` // tcp
	Enabled   int       `json:"enabled" gorm:"default:1"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ServerStatusMonitor) TableName() string {
	return "server_status_monitors"
}
