package model

import (
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
)

// ProcessMonitorRule 进程/端口监控规则（后台集中配置，按设备下发到客户端）
// source_type 为空或 peers 表示单设备规则；device_group / ab_tags 表示集合规则

type ProcessMonitorRule struct {
	RowId           uint   `json:"row_id" gorm:"primaryKey"`
	UserId          uint   `json:"user_id" gorm:"default:0;not null;index"`             // 创建者
	PeerId          string `json:"peer_id" gorm:"size:128;not null;default:'';index"`   // 单设备规则的目标 peer id
	SourceType      string `json:"source_type" gorm:"size:16;not null;default:''"`      // '' | peers | device_group | ab_tags
	SourceId        string `json:"source_id" gorm:"size:128;not null;default:''"`       // 设备组ID或地址簿标签
	SourceName      string `json:"source_name" gorm:"size:128;not null;default:''"`     // 展示名称（设备组名/标签名）
	Name            string `json:"name" gorm:"size:128;not null;default:''"`            // 监控项展示名
	Type            string `json:"type" gorm:"size:16;not null;default:'process'"`      // process | port
	Target          string `json:"target" gorm:"size:255;not null;default:''"`          // 进程名(如 notepad.exe) 或 端口(如 8080)
	Interval        int    `json:"interval" gorm:"default:30"`                          // 检测间隔(秒)
	DownThreshold   int    `json:"down_threshold" gorm:"default:300"`                   // 连续 down 多少秒后触发告警
	AlertConfigId   uint   `json:"alert_config_id" gorm:"default:0"`                    // 关联告警规则(复用 alert_config)；0=不告警
	Enabled         int    `json:"enabled" gorm:"default:1"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (ProcessMonitorRule) TableName() string {
	return "process_monitor_rules"
}

// ProcessMonitorRulePeer 集合规则与设备的关联及单设备覆盖配置
// overrides 为空对象 {} 时完全继承父规则；非空字段覆盖父规则
type ProcessMonitorRulePeer struct {
	RowId     uint                  `json:"row_id" gorm:"primaryKey"`
	RuleId    uint                  `json:"rule_id" gorm:"not null;index"`
	PeerId    string                `json:"peer_id" gorm:"size:128;not null;default:'';index"`
	Overrides custom_types.AutoJson `json:"overrides" gorm:"not null;" swaggertype:"object"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

func (ProcessMonitorRulePeer) TableName() string {
	return "process_monitor_rule_peers"
}

// ProcessMonitorStatus 设备上报的监控项实时状态
type ProcessMonitorStatus struct {
	RowId     uint      `json:"row_id" gorm:"primaryKey"`
	PeerId    string    `json:"peer_id" gorm:"size:128;not null;default:'';index"`
	RuleId    uint      `json:"rule_id" gorm:"default:0;index"`
	Name      string    `json:"name" gorm:"size:128;not null;default:''"`
	Type      string    `json:"type" gorm:"size:16;not null;default:''"`
	Target    string    `json:"target" gorm:"size:255;not null;default:''"`
	Running   int       `json:"running" gorm:"default:0"` // 1=运行 0=未运行
	LastSeen  int64     `json:"last_seen"`                 // 最近一次上报 running=1 的时间戳
	DownSince int64     `json:"down_since"`                // 首次检测到 running=0 的时间戳；恢复后清零
	Alerted   int       `json:"alerted" gorm:"default:0"`  // 是否已发送告警(避免重复发送)
	UpdatedAt time.Time `json:"updated_at"`
}

func (ProcessMonitorStatus) TableName() string {
	return "process_monitor_status"
}
