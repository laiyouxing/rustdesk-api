package service

import (
	"strconv"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

// 后台可配置参数的键名
const (
	// SettingKeyLogMaxDays 日志保留天数（按天轮转）
	SettingKeyLogMaxDays = "log.max_days"
	// SettingKeyAuditRetentionDays 审计记录保留天数
	SettingKeyAuditRetentionDays = "audit.retention_days"
)

// SettingService 系统参数读写。
//
// 优先级：system_settings 表 > config.yaml > 内置默认值。
// 表里没有记录（或值非法）时回退到配置文件，保证老部署行为不变。
type SettingService struct {
}

// Get 读取字符串参数，第二个返回值表示表中是否存在。
func (s *SettingService) Get(key string) (string, bool) {
	if DB == nil {
		return "", false
	}
	var row model.SystemSetting
	// 用结构体条件而不是手写 SQL：key 在 MySQL 里是保留字，
	// 手写反引号会破坏 postgres/sqlite 兼容性，交给 GORM 生成正确引号。
	if err := DB.Where(&model.SystemSetting{Key: key}).First(&row).Error; err != nil {
		return "", false
	}
	return row.Value, true
}

// Set 写入（不存在则创建）参数。
func (s *SettingService) Set(key, value string) error {
	var row model.SystemSetting
	err := DB.Where(&model.SystemSetting{Key: key}).First(&row).Error
	if err != nil {
		return DB.Create(&model.SystemSetting{Key: key, Value: value}).Error
	}
	return DB.Model(&model.SystemSetting{}).Where("id = ?", row.Id).Update("value", value).Error
}

// GetInt 读取整数参数，值非法或不存在时返回 (0,false)。
func (s *SettingService) GetInt(key string) (int, bool) {
	v, ok := s.Get(key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// EffectiveLogMaxDays 日志保留天数：DB 设置 > config.yaml > 默认 30。
func EffectiveLogMaxDays() int {
	if DB != nil && AllService != nil && AllService.SettingService != nil {
		if n, ok := AllService.SettingService.GetInt(SettingKeyLogMaxDays); ok && n > 0 {
			return n
		}
	}
	if Config != nil {
		return Config.Logger.EffectiveMaxDays()
	}
	return 30
}

// EffectiveAuditRetentionDays 审计保留天数：DB 设置 > config.yaml > 默认 90。
func EffectiveAuditRetentionDays() int {
	if DB != nil && AllService != nil && AllService.SettingService != nil {
		if n, ok := AllService.SettingService.GetInt(SettingKeyAuditRetentionDays); ok && n > 0 {
			return n
		}
	}
	if Config != nil {
		return Config.Audit.EffectiveRetentionDays()
	}
	return 90
}
