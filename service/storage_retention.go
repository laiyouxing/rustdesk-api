package service

import (
	"os"
	"path/filepath"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/lib/logger"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

// StorageRetentionService 「存储管理」的保留策略执行器。
//
// 背景：审计记录（连接/文件/直连上报）已有 90 天自动清理，但站内消息、
// 登录日志、操作日志此前**从不清理**，量大了会长期占用数据库。
// 这里把它们纳入同一套巡检，且**每类可单独配置天数**（0 = 不清理），
// 由后台「存储管理」页维护。
//
// 注意：
//   - 只删超过保留期限的记录，近期数据不受影响；
//   - 某类配置为 0 时完全跳过，绝不"顺手清理"；
//   - 审计记录另有配置项（audit.retention_days），本服务只额外处理下面三类。
type StorageRetentionService struct {
}

// retentionTarget 一类可清理的数据
type retentionTarget struct {
	Key   string             // 设置键
	Name  string             // 展示名
	Model func() interface{} // 构造模型实例供 gorm 定位表
	Days  func() int         // 取生效天数
	Guard func() bool        // 额外前置条件（可选）
}

// StorageRetentionTargets 返回所有由本服务负责的清理目标。
// 导出供后台页面展示"当前生效状态"。
func StorageRetentionTargets() []retentionTarget {
	return []retentionTarget{
		{
			Key:   SettingKeyStationMessageRetentionDays,
			Name:  "站内消息",
			Model: func() interface{} { return &model.StationMessage{} },
			Days:  EffectiveStationMessageRetentionDays,
		},
		{
			Key:   SettingKeyLoginLogRetentionDays,
			Name:  "登录日志",
			Model: func() interface{} { return &model.LoginLog{} },
			Days:  EffectiveLoginLogRetentionDays,
		},
		{
			Key:   SettingKeyOpLogRetentionDays,
			Name:  "操作日志",
			Model: func() interface{} { return &model.AccountOpLog{} },
			Days:  EffectiveOpLogRetentionDays,
		},
		{
			Key:   SettingKeyOpLogRetentionDays,
			Name:  "地址簿操作日志",
			Model: func() interface{} { return &model.AddressBookOpLog{} },
			Days:  EffectiveOpLogRetentionDays,
		},
	}
}

// PruneNow 立即按当前策略清理一次（供"保存后立即清理"使用），返回删除条数。
func (s *StorageRetentionService) PruneNow() int64 {
	return s.pruneOnce()
}

// StartSweep 定期巡检（启动时先跑一次）。
func (s *StorageRetentionService) StartSweep() {
	s.pruneOnce()
	ticker := time.NewTicker(AuditRetentionInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.pruneOnce()
	}
}

func (s *StorageRetentionService) pruneOnce() int64 {
	total := int64(0)
	for _, t := range StorageRetentionTargets() {
		days := t.Days()
		if days <= 0 {
			// 0 = 不自动清理，跳过
			continue
		}
		cutoff := time.Now().AddDate(0, 0, -days)
		n := deleteOlderThan(t.Model(), t.Name, cutoff)
		if n > 0 {
			global.Logger.Infof("[StorageRetention] 已清理超过 %d 天的%s %d 条（cutoff=%s）",
				days, t.Name, n, cutoff.Format("2006-01-02 15:04:05"))
		}
		total += n
	}
	return total
}

// deleteOlderThan 删除某张表中 created_at 早于 cutoff 的记录。
func deleteOlderThan(m interface{}, name string, cutoff time.Time) int64 {
	res := DB.Where("created_at < ?", cutoff).Delete(m)
	if res.Error != nil {
		global.Logger.Warnf("[StorageRetention] 清理 %s 失败: %v", name, res.Error)
		return 0
	}
	return res.RowsAffected
}

// StorageTableStat 单张表的占用统计
type StorageTableStat struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// StorageStats 存储占用总览
type StorageStats struct {
	Tables       []StorageTableStat `json:"tables"`
	DBFile       string             `json:"db_file"`
	DBSizeBytes  int64              `json:"db_size_bytes"`
	LogDir       string             `json:"log_dir"`
	LogSizeBytes int64              `json:"log_size_bytes"`
	LogFileCount int                `json:"log_file_count"`
}

// CollectStorageStats 统计各表行数与磁盘占用。
//
// 行数用 COUNT(*)（各表都有 created_at 索引以外的代价，但只在前端打开
// 页面时调用，可接受）。磁盘部分只统计 SQLite 主库文件与日志目录；
// 其它数据库类型下 DBFile/DBSize 留空，前端会隐藏对应项。
func (s *StorageRetentionService) CollectStorageStats() *StorageStats {
	st := &StorageStats{Tables: []StorageTableStat{}}

	type target struct {
		Key   string
		Name  string
		Model interface{}
	}
	targets := []target{
		{"audit_conn", "连接日志", &model.AuditConn{}},
		{"audit_file", "文件传输日志", &model.AuditFile{}},
		{"punch_direct_ok", "直连上报", &model.PunchDirectOk{}},
		{"station_message", "站内消息", &model.StationMessage{}},
		{"login_log", "登录日志", &model.LoginLog{}},
		{"account_op_log", "账户操作日志", &model.AccountOpLog{}},
		{"address_book_op_log", "地址簿操作日志", &model.AddressBookOpLog{}},
		{"peer", "设备", &model.Peer{}},
		{"user_token", "登录凭证", &model.UserToken{}},
	}
	for _, t := range targets {
		var n int64
		if err := DB.Model(t.Model).Count(&n).Error; err != nil {
			continue
		}
		st.Tables = append(st.Tables, StorageTableStat{Key: t.Key, Name: t.Name, Count: n})
	}

	// SQLite 主库文件：用 PRAGMA database_list 拿到真实路径，避免硬编码
	// data/rustdeskapi.db（部署路径可能不同）。
	var rows []struct {
		Seq  int    `gorm:"column:seq"`
		Name string `gorm:"column:name"`
		File string `gorm:"column:file"`
	}
	if DB.Raw("PRAGMA database_list").Scan(&rows).Error == nil {
		for _, r := range rows {
			if r.Name == "main" && r.File != "" {
				st.DBFile = r.File
				if fi, err := os.Stat(r.File); err == nil {
					st.DBSizeBytes = fi.Size()
				}
				break
			}
		}
	}

	// 日志目录：从当前日志文件反推
	if p := logger.CurrentLogPath(); p != "" {
		st.LogDir = filepath.Dir(p)
		if entries, err := os.ReadDir(st.LogDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if fi, err := e.Info(); err == nil {
					st.LogSizeBytes += fi.Size()
					st.LogFileCount++
				}
			}
		}
	}
	return st
}
