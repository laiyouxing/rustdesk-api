package service

import (
	"time"

	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

// AuditRetentionService 审计记录保留策略。
//
// 背景：audit_conn / audit_file / punch_direct_ok 三张表此前只有后台手动
// 删除，没有自动过期，会话量一大就会无限增长。这里按保留天数（默认 90 天，
// 由 config.yaml 的 audit.retention-days 控制）定期清理。
//
// 注意：
//   - 只删"超过保留期限"的旧记录，当天/近期记录不受影响；
//   - 后台手动删除仍然可用，本服务只是兜底，不做任何"容量驱动"的删新操作；
//   - 审计记录是合规留痕，保留期不要设置得过短（默认 90 天）。
type AuditRetentionService struct {
}

// AuditRetentionInterval 清理检查间隔。
// 保留期以天为单位，半天检查一次足够，避免频繁全表扫描。
const AuditRetentionInterval = 12 * time.Hour

// StartAuditRetentionSweep 后台定期清理过期审计记录（启动时先跑一次）
func (s *AuditRetentionService) StartAuditRetentionSweep() {
	s.pruneOnce()
	ticker := time.NewTicker(AuditRetentionInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.pruneOnce()
	}
}

// pruneOnce 执行一次清理，返回本轮删除的总条数。
func (s *AuditRetentionService) pruneOnce() int64 {
	days := 0
	if Config != nil {
		days = Config.Audit.EffectiveRetentionDays()
	} else {
		days = 90
	}
	cutoff := time.Now().AddDate(0, 0, -days)

	total := int64(0)
	// 连接日志
	total += deleteAuditOlderThan(&model.AuditConn{}, "audit_conn", cutoff)
	// 文件传输日志
	total += deleteAuditOlderThan(&model.AuditFile{}, "audit_file", cutoff)
	// 直连上报（合规留痕）
	total += deleteAuditOlderThan(&model.PunchDirectOk{}, "punch_direct_ok", cutoff)

	if total > 0 {
		global.Logger.Infof("[AuditRetention] 已清理超过 %d 天的审计记录共 %d 条（cutoff=%s）",
			days, total, cutoff.Format("2006-01-02 15:04:05"))
	}
	return total
}

// deleteAuditOlderThan 删除某张审计表中 created_at 早于 cutoff 的记录。
func deleteAuditOlderThan(m interface{}, table string, cutoff time.Time) int64 {
	res := DB.Where("created_at < ?", cutoff).Delete(m)
	if res.Error != nil {
		global.Logger.Warnf("[AuditRetention] 清理 %s 失败: %v", table, res.Error)
		return 0
	}
	return res.RowsAffected
}
