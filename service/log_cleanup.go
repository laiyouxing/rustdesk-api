package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/global"
)

// LogCleanupService 日志自动清理服务。
// 当日志文件所在磁盘剩余空间 < MinFreePercent% 时，按修改时间从旧到新删除日志文件，
// 直到剩余空间恢复或没有可删除的日志文件为止；并将清理动作写入日志文件。
type LogCleanupService struct {
}

// MinFreePercent 磁盘剩余空间警戒线（%）。低于该值触发清理。
const MinFreePercent = 10

// CleanupInterval 清理检查间隔
const CleanupInterval = 10 * time.Minute

// StartLogCleanupSweep 后台定时检查并清理日志目录
func (s *LogCleanupService) StartLogCleanupSweep() {
	// 启动后先执行一次
	s.checkAndCleanup()
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.checkAndCleanup()
	}
}

// checkAndCleanup 检查日志目录磁盘剩余空间并触发清理
func (s *LogCleanupService) checkAndCleanup() {
	path := global.Config.Logger.Path
	if path == "" {
		// 未配置日志文件（仅 stdout），无从清理
		return
	}
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}

	freePercent, err := diskFreePercent(dir)
	if err != nil {
		global.Logger.Warnf("[LogCleanup] 获取磁盘剩余空间失败: %v", err)
		return
	}
	if freePercent >= float64(MinFreePercent) {
		return
	}
	global.Logger.Warnf("[LogCleanup] 日志目录 %s 所在磁盘剩余空间 %.1f%% 低于 %d%%，开始清理日志", dir, freePercent, MinFreePercent)

	cleaned, freedBytes := s.cleanupDir(dir, path)
	afterPercent, _ := diskFreePercent(dir)
	msg := fmt.Sprintf("[LogCleanup] 清理完成：删除 %d 个日志文件，释放 %.1f MB，剩余空间 %.1f%%",
		cleaned, float64(freedBytes)/1024/1024, afterPercent)
	if cleaned > 0 {
		global.Logger.Warnf("%s", msg)
	} else {
		global.Logger.Warnf("%s（无更多日志文件可清理）", msg)
	}
}

// cleanupDir 删除目录下最旧的日志文件（排除正在写入的当前日志文件），
// 直到磁盘剩余空间恢复到阈值以上，或没有可删除文件。
// 返回 (删除文件数, 释放字节数)
func (s *LogCleanupService) cleanupDir(dir, currentLog string) (int, int64) {
	curBase := filepath.Base(currentLog)
	var removed int
	var freed int64

	for i := 0; i < 100; i++ { // 防止极端情况下死循环
		freePercent, err := diskFreePercent(dir)
		if err != nil || freePercent >= float64(MinFreePercent) {
			break
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			global.Logger.Warnf("[LogCleanup] 读取日志目录失败: %v", err)
			break
		}
		// 收集可删除的日志文件（排除当前正在写入的文件）
		var candidates []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if e.Name() == curBase {
				continue
			}
			// 只处理日志类文件：.log / .txt / .log.* 等
			ext := filepath.Ext(e.Name())
			if ext != ".log" && ext != ".txt" && ext != ".out" {
				continue
			}
			candidates = append(candidates, filepath.Join(dir, e.Name()))
		}
		if len(candidates) == 0 {
			break
		}
		// 按修改时间从旧到新排序
		type candidate struct {
			path string
			mod  time.Time
			size int64
		}
		var cs []candidate
		for _, p := range candidates {
			info, err := os.Stat(p)
			if err != nil {
				continue
			}
			cs = append(cs, candidate{path: p, mod: info.ModTime(), size: info.Size()})
		}
		sort.Slice(cs, func(a, b int) bool { return cs[a].mod.Before(cs[b].mod) })

		deleted := false
		for _, c := range cs {
			if err := os.Remove(c.path); err != nil {
				global.Logger.Warnf("[LogCleanup] 删除日志文件 %s 失败: %v", c.path, err)
				continue
			}
			removed++
			freed += c.size
			deleted = true
			global.Logger.Warnf("[LogCleanup] 已删除日志文件 %s（%.1f MB）", c.path, float64(c.size)/1024/1024)
			// 删除一个后重新检查，达到阈值即可停止
			break
		}
		if !deleted {
			break
		}
	}
	return removed, freed
}
