package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/lib/logger"
)

// LogCleanupService 磁盘压力兜底清理。
//
// 职责边界（重要）：
//   - **按天轮转与按天数保留**由 lib/logger 的写入器负责：每天一个文件
//     （{prefix}-{YYYY-MM-DD}{ext}），跨天时删除超过 logger.max-days 的旧文件。
//     正常运行时磁盘占用是有界的，不依赖本服务。
//   - **本服务只做兜底**：当磁盘剩余空间低于 MinFreePercent 时，额外删除
//     最旧的历史日志文件，尽量把空间抢救回来。它不承担日常保留策略。
//
// 与旧实现的区别：
//   - 旧实现打开单个永不轮转的文件，却跳过"当前正在写入的文件"，
//     导致真正增长的文件永远删不掉，清理实际无效；
//   - 旧实现按 .log/.txt/.out 删除同目录所有文件，会误删其它程序的文件。
//     现在只删符合本进程日志命名的文件（logger.IsOwnLogFile）。
type LogCleanupService struct {
}

// MinFreePercent 磁盘剩余空间警戒线（%）。低于该值触发兜底清理。
const MinFreePercent = 10

// CleanupInterval 清理检查间隔
const CleanupInterval = 10 * time.Minute

// StartLogCleanupSweep 后台定时检查磁盘并兜底清理
func (s *LogCleanupService) StartLogCleanupSweep() {
	// 启动后先执行一次
	s.checkAndCleanup()
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.checkAndCleanup()
	}
}

// checkAndCleanup 检查日志目录磁盘剩余空间并触发兜底清理
func (s *LogCleanupService) checkAndCleanup() {
	// 以"当前正在写入的文件"为准：按天轮转后配置里的 path 已不是实际文件名
	cur := logger.CurrentLogPath()
	if cur == "" {
		// 未配置日志文件（仅 stdout），无从清理
		return
	}
	dir := filepath.Dir(cur)
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
	global.Logger.Warnf("[LogCleanup] 日志目录 %s 所在磁盘剩余空间 %.1f%% 低于 %d%%，开始兜底清理历史日志", dir, freePercent, MinFreePercent)

	cleaned, freedBytes := s.cleanupDir(dir, cur)
	afterPercent, _ := diskFreePercent(dir)
	msg := fmt.Sprintf("[LogCleanup] 清理完成：删除 %d 个历史日志文件，释放 %.1f MB，剩余空间 %.1f%%",
		cleaned, float64(freedBytes)/1024/1024, afterPercent)
	if cleaned > 0 {
		global.Logger.Warnf("%s", msg)
	} else {
		global.Logger.Warnf("%s（无更多历史日志可清理）", msg)
	}
}

// cleanupDir 删除目录下最旧的历史日志文件（排除当前正在写入的文件），
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
		// 收集可删除的日志文件：只认本进程的按天日志，且排除当前文件
		type candidate struct {
			path string
			mod  time.Time
			size int64
		}
		var cs []candidate
		for _, e := range entries {
			if e.IsDir() || e.Name() == curBase {
				continue
			}
			if !logger.IsOwnLogFile(e.Name()) {
				continue
			}
			p := filepath.Join(dir, e.Name())
			info, err := os.Stat(p)
			if err != nil {
				continue
			}
			cs = append(cs, candidate{path: p, mod: info.ModTime(), size: info.Size()})
		}
		if len(cs) == 0 {
			break
		}
		// 按修改时间从旧到新排序，每次只删一个，删完重新检查空间
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
			global.Logger.Warnf("[LogCleanup] 已删除历史日志文件 %s（%.1f MB）", c.path, float64(c.size)/1024/1024)
			break
		}
		if !deleted {
			break
		}
	}
	return removed, freed
}
