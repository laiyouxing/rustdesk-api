package config

// DefaultLogMaxDays 日志默认保留天数（按天轮转）。
const DefaultLogMaxDays = 30

type Logger struct {
	Path         string
	Level        string
	ReportCaller bool `mapstructure:"report-caller"`
	// MaxDays 日志保留天数（按天轮转，每天一个文件）。
	// <=0 时使用 DefaultLogMaxDays。
	// 轮转产生 {prefix}-{YYYY-MM-DD}{ext}，超过该天数的旧文件在轮转时删除。
	MaxDays int `mapstructure:"max-days"`
}

// EffectiveMaxDays 返回实际生效的保留天数。
func (l *Logger) EffectiveMaxDays() int {
	if l.MaxDays <= 0 {
		return DefaultLogMaxDays
	}
	return l.MaxDays
}
