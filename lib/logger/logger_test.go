package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 按天轮转：写入后文件名应为 {prefix}-{YYYY-MM-DD}{ext}
func TestDailyFileNamesByDate(t *testing.T) {
	dir := t.TempDir()
	w := &dailyFile{dir: dir, prefix: "log", ext: ".txt", maxDays: 30}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	defer w.Close()

	want := filepath.Join(dir, "log-"+today()+".txt")
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("expected today's file %s: %v", want, err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected content: %q", string(data))
	}
	// CurrentPath 必须指向正在写的文件（清理模块靠它跳过）
	if got := w.CurrentPath(); got != want {
		t.Fatalf("CurrentPath = %s, want %s", got, want)
	}
}

// 跨天时应切换到新文件（手工改 curDate 模拟）
func TestDailyFileRotatesOnDateChange(t *testing.T) {
	dir := t.TempDir()
	w := &dailyFile{dir: dir, prefix: "log", ext: ".log", maxDays: 30}
	if _, err := w.Write([]byte("a")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// 模拟日期变化：把 curDate 改成昨天，下次写入应重新轮转回今天
	w.mu.Lock()
	w.curDate = "2000-01-01"
	w.mu.Unlock()
	if _, err := w.Write([]byte("b")); err != nil {
		t.Fatalf("write after date change: %v", err)
	}
	defer w.Close()

	files, _ := os.ReadDir(dir)
	// 今天一个文件；2000-01-01 那个是模拟值，不会被真实轮转创建
	count := 0
	for _, f := range files {
		if f.Name() == "log-"+today()+".log" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 file for today, got %d (files=%v)", count, files)
	}
}

// 超过保留天数的旧日志应被删除；同目录里其它文件不能动
func TestPruneRemovesOnlyOwnOldLogs(t *testing.T) {
	dir := t.TempDir()
	oldDate := time.Now().AddDate(0, 0, -40).Format(dateLayout) // 40 天前，> maxDays 30
	oldLog := filepath.Join(dir, "log-"+oldDate+".txt")
	recentDate := time.Now().AddDate(0, 0, -3).Format(dateLayout) // 3 天前，应保留
	recentLog := filepath.Join(dir, "log-"+recentDate+".txt")
	unrelated := filepath.Join(dir, "other.txt")   // 别的程序的文件，必须保留
	nonDate := filepath.Join(dir, "log-notes.txt") // 同前缀但不是日期命名，必须保留

	for _, p := range []string{oldLog, recentLog, unrelated, nonDate} {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatalf("setup %s: %v", p, err)
		}
	}

	w := &dailyFile{dir: dir, prefix: "log", ext: ".txt", maxDays: 30}
	if _, err := w.Write([]byte("today")); err != nil { // 写入触发 prune
		t.Fatalf("write: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatalf("old log %s should be pruned", filepath.Base(oldLog))
	}
	for _, p := range []string{recentLog, unrelated, nonDate} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s should be kept: %v", filepath.Base(p), err)
		}
	}
}

// 旧版单文件日志应在启动时被改名归入按天命名，避免变成永不清理的孤儿文件
func TestMigrateLegacyFile(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(legacy, []byte("old"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	migrateLegacyFile(legacy)

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file should be renamed away")
	}
	// 归到 mtime 那天的日期文件
	want := fileNameFor(dir, "log", ".txt", time.Now().Format(dateLayout))
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected migrated file %s: %v", filepath.Base(want), err)
	}
}

// 目标日期文件已存在时不能覆盖（否则会丢已有日志）
func TestMigrateLegacyFileNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(legacy, []byte("legacy"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	existing := fileNameFor(dir, "log", ".txt", time.Now().Format(dateLayout))
	if err := os.WriteFile(existing, []byte("existing"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	migrateLegacyFile(legacy)

	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy file must be kept when target exists: %v", err)
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "existing" {
		t.Fatalf("existing file must not be overwritten, got %q", string(data))
	}
}

// IsOwnLogFile 只认本前缀 + 日期命名 + 本扩展名
func TestIsOwnLogFile(t *testing.T) {
	saved := curWriter
	curWriter = &dailyFile{dir: ".", prefix: "log", ext: ".txt"}
	defer func() { curWriter = saved }()

	cases := map[string]bool{
		"log-2026-10-29.txt": true,
		"log-2020-01-01.txt": true,
		"log-notes.txt":      false, // 非日期
		"other-2026-10-29.txt": false, // 前缀不符
		"log-2026-10-29.log": false,  // 扩展名不符
	}
	for name, want := range cases {
		if got := IsOwnLogFile(name); got != want {
			t.Errorf("IsOwnLogFile(%q) = %v, want %v", name, got, want)
		}
	}
}
