package config

// DefaultAuditRetentionDays 审计记录默认保留天数。
const DefaultAuditRetentionDays = 90

// Audit 审计记录保留策略。
//
// 覆盖三张审计表：audit_conn（连接日志）、audit_file（文件传输日志）、
// punch_direct_ok（直连上报）。后台手动删除仍然可用，这里只是加一条
// 自动过期的兜底，避免表无限增长。
type Audit struct {
	// RetentionDays 保留天数，<=0 时使用 DefaultAuditRetentionDays；
	// 设为负数以外的大值可延长保留。等于 0 走默认值。
	RetentionDays int `mapstructure:"retention-days"`
}

// EffectiveRetentionDays 返回实际生效的保留天数。
func (a *Audit) EffectiveRetentionDays() int {
	if a.RetentionDays <= 0 {
		return DefaultAuditRetentionDays
	}
	return a.RetentionDays
}
