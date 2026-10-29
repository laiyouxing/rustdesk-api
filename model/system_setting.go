package model

// SystemSetting 后台可配置的系统参数（键值对）。
//
// 目前用于「日志保留天数」「审计记录保留天数」这类需要后台界面可改、
// 且改完立即生效的参数。
//
// 优先级：system_settings 表 > config.yaml > 代码内置默认值。
// 之所以不直接改 config.yaml：那是有大量注释的运维文件，程序回写容易
// 破坏注释与格式；而这些参数本身适合热更新，不需要重启。
type SystemSetting struct {
	IdModel
	Key   string `json:"key" gorm:"size:64;not null;default:'';uniqueIndex"`
	Value string `json:"value" gorm:"type:text"`
	TimeModel
}

func (SystemSetting) TableName() string {
	return "system_settings"
}
