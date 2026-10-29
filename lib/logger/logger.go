package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	log "github.com/sirupsen/logrus"
)

const (
	DebugMode   = "debug"
	ReleaseMode = "release"
)

// dateLayout 按天轮转使用的日期格式
const dateLayout = "2006-01-02"

type Config struct {
	Path         string
	Level        string
	ReportCaller bool
	// MaxDays 日志保留天数（按天轮转）。<=0 表示不按天清理。
	MaxDays int
}

// dailyFile 按天轮转的日志写入器。
//
// 为什么需要：原实现用 O_APPEND 打开**单个**文件且永不轮转，而
// LogCleanupService 又明确跳过"当前正在写入的文件"——真正无限增长的那个
// 文件永远删不掉，磁盘清理形同虚设。改为每天一个文件后：
//   - 增长的永远是"今天"这个文件；
//   - 清理模块可以安全删除历史文件；
//   - 本写入器自身在跨天时顺带按 MaxDays 清理过期文件，不依赖磁盘压力。
//
// 文件名：{dir}/{prefix}-{YYYY-MM-DD}{ext}
// 例如 path=./runtime/log.txt → ./runtime/log-2026-10-29.txt
type dailyFile struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	ext     string
	maxDays int

	file    *os.File
	curDate string
}

func today() string { return time.Now().Format(dateLayout) }

// splitLogPath 把配置的日志路径拆成 (目录, 前缀, 扩展名)。
// 配置路径本身不再作为实际文件名，只用来确定目录与命名前缀。
func splitLogPath(path string) (dir, prefix, ext string) {
	dir = filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	base := filepath.Base(path)
	ext = filepath.Ext(base)
	prefix = strings.TrimSuffix(base, ext)
	if prefix == "" {
		prefix = "log"
	}
	if ext == "" {
		ext = ".log"
	}
	return
}

func (w *dailyFile) fileName(date string) string {
	return filepath.Join(w.dir, fmt.Sprintf("%s-%s%s", w.prefix, date, w.ext))
}

// CurrentPath 返回当前正在写入的文件路径（供磁盘清理模块跳过它）。
func (w *dailyFile) CurrentPath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.curDate == "" {
		return w.fileName(today())
	}
	return w.fileName(w.curDate)
}

func (w *dailyFile) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	d := today()
	if w.file == nil || d != w.curDate {
		if err := w.rotateLocked(d); err != nil {
			return 0, err
		}
	}
	return w.file.Write(p)
}

func (w *dailyFile) rotateLocked(date string) error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	f, err := os.OpenFile(fileNameFor(w.dir, w.prefix, w.ext, date), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	w.file = f
	w.curDate = date
	w.pruneLocked()
	return nil
}

// pruneLocked 删除超过 MaxDays 的旧日志。
//
// 只认本系统自己的命名（{prefix}-{YYYY-MM-DD}{ext}），避免误删同目录下
// 其它程序的 .log/.txt 文件——这是原清理逻辑的一个隐患。
func (w *dailyFile) pruneLocked() {
	if w.maxDays <= 0 {
		return
	}
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -w.maxDays)
	prefixDash := w.prefix + "-"
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefixDash) || !strings.HasSuffix(name, w.ext) {
			continue
		}
		dateStr := strings.TrimSuffix(strings.TrimPrefix(name, prefixDash), w.ext)
		t, perr := time.ParseInLocation(dateLayout, dateStr, time.Local)
		if perr != nil {
			continue // 不是按日期命名的文件，不动
		}
		if t.Before(cutoff) {
			_ = os.Remove(filepath.Join(w.dir, name))
		}
	}
}

func (w *dailyFile) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func fileNameFor(dir, prefix, ext, date string) string {
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, date, ext))
}

// curWriter 当前进程的按天轮转写入器（未配置文件日志时为 nil）。
var curWriter *dailyFile

// CurrentLogPath 返回当前正在写入的日志文件路径。
// 未配置文件日志（仅 stdout）时返回空字符串。
func CurrentLogPath() string {
	if curWriter == nil {
		return ""
	}
	return curWriter.CurrentPath()
}

// IsOwnLogFile 判断文件名是否是本进程按天轮转产生的日志文件
// （形如 {prefix}-{YYYY-MM-DD}{ext}）。
//
// 供磁盘清理模块限定删除范围：原实现按扩展名删同目录所有 .log/.txt/.out，
// 若日志目录里放了别的程序的文件会被误删。
func IsOwnLogFile(name string) bool {
	if curWriter == nil {
		return false
	}
	return curWriter.isOwn(name)
}

func (w *dailyFile) isOwn(name string) bool {
	prefixDash := w.prefix + "-"
	if !strings.HasPrefix(name, prefixDash) || !strings.HasSuffix(name, w.ext) {
		return false
	}
	dateStr := strings.TrimSuffix(strings.TrimPrefix(name, prefixDash), w.ext)
	_, err := time.ParseInLocation(dateLayout, dateStr, time.Local)
	return err == nil
}

// migrateLegacyFile 把旧版单文件日志改名为按天命名，交给轮转体系接管。
//
// 旧实现写 {path}（如 runtime/log.txt）。改用按天后若放任不管，那个文件
// 既不会再增长、也不会被保留策略清理（清理只认日期命名），会永久占空间。
// 这里按它的 mtime 归到对应日期；目标已存在则不覆盖，失败静默忽略
// （迁移失败最多是多留一个旧文件，不应阻塞启动）。
func migrateLegacyFile(path string) {
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	dir, prefix, ext := splitLogPath(path)
	dst := fileNameFor(dir, prefix, ext, info.ModTime().Format(dateLayout))
	if dst == path {
		return
	}
	if _, err := os.Stat(dst); err == nil {
		return
	}
	_ = os.Rename(path, dst)
}

// SetMaxDays 运行时修改日志保留天数（后台界面可配），并立即按新值清理一次。
// 未配置文件日志时为空操作。
func SetMaxDays(n int) {
	if curWriter == nil {
		return
	}
	curWriter.mu.Lock()
	defer curWriter.mu.Unlock()
	curWriter.maxDays = n
	curWriter.pruneLocked()
}

// CurrentMaxDays 返回当前生效的保留天数（未配置文件日志时返回 0）。
func CurrentMaxDays() int {
	if curWriter == nil {
		return 0
	}
	curWriter.mu.Lock()
	defer curWriter.mu.Unlock()
	return curWriter.maxDays
}

func New(c *Config) *log.Logger {
	log.SetFormatter(&nested.Formatter{
		// HideKeys:        true,
		TimestampFormat: "[2006-01-02 15:04:05]",
		NoColors:        true,
		NoFieldsColors:  true,
		//FieldsOrder:     []string{"name", "age"},
	})

	// 日志文件：按天轮转
	var write io.Writer
	if c.Path != "" {
		migrateLegacyFile(c.Path) // 旧版单文件日志归入按天命名
		dir, prefix, ext := splitLogPath(c.Path)
		w := &dailyFile{
			dir:     dir,
			prefix:  prefix,
			ext:     ext,
			maxDays: c.MaxDays,
		}
		// 启动即创建当天文件，尽早暴露"目录不存在/无权限"这类问题
		if err := w.rotateLocked(today()); err != nil {
			// 保持与原实现一致：日志文件打不开直接 panic（否则会静默丢日志）
			panic(fmt.Sprintf("open log file fail: %v", err))
		}
		curWriter = w
		write = io.MultiWriter(w, os.Stdout)
	} else {
		write = os.Stdout
	}

	log.SetOutput(write)

	log.SetReportCaller(c.ReportCaller)

	level, err2 := log.ParseLevel(c.Level)
	if err2 != nil {
		level = log.DebugLevel
	}
	log.SetLevel(level)

	return log.StandardLogger()
}
