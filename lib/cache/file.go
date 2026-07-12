package cache

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type FileCache struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	Dir   string
}

func (fc *FileCache) getLock(key string) *sync.Mutex {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.locks == nil {
		fc.locks = make(map[string]*sync.Mutex)
	}
	if _, ok := fc.locks[key]; !ok {
		fc.locks[key] = new(sync.Mutex)
	}
	return fc.locks[key]
}

// fileItem 是落盘文件的内部封装，把过期时间戳写进文件内容，
// 避免依赖文件 mtime（未来时间脆弱，易被备份/同步/杀软重置导致缓存永远 miss）
type fileItem struct {
	Exp  int64  `json:"exp"`  // 过期 unix 时间戳；<=0 表示使用 MaxTimeOut
	Data string `json:"data"` // EncodeValue 后的 JSON 字符串
}

func (c *FileCache) fileName(key string) string {
	f := c.Dir + string(os.PathSeparator) + fmt.Sprintf("%x", md5.Sum([]byte(key)))
	return f
}

// getValue 读取落盘值；文件不存在/已过期/损坏时返回空串（过滤错误）
func (c *FileCache) getValue(key string) string {
	f := c.fileName(key)
	lock := c.getLock(f)
	lock.Lock()
	defer lock.Unlock()

	data, err := os.ReadFile(f)
	if err != nil {
		return ""
	}
	var item fileItem
	if err := json.Unmarshal(data, &item); err != nil {
		// 文件损坏（含旧格式），删除后视为未命中
		os.Remove(f)
		return ""
	}
	if item.Exp > 0 && time.Now().Unix() >= item.Exp {
		os.Remove(f)
		return ""
	}
	return item.Data
}

// Get 读取缓存；未命中时 value 保持零值且不返回错误（与 SimpleCache 行为一致）
func (c *FileCache) Get(key string, value interface{}) error {
	data := c.getValue(key)
	if data == "" {
		return nil
	}
	return DecodeValue(data, value)
}

func (c *FileCache) saveValue(key string, value string, exp int) error {
	f := c.fileName(key)
	lock := c.getLock(f)
	lock.Lock()
	defer lock.Unlock()

	if exp <= 0 {
		exp = MaxTimeOut
	}
	item := fileItem{
		Exp:  time.Now().Add(time.Duration(exp) * time.Second).Unix(),
		Data: value,
	}
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return os.WriteFile(f, b, 0644)
}

func (c *FileCache) Set(key string, value interface{}, exp int) error {
	str, err := EncodeValue(value)
	if err != nil {
		return err
	}
	return c.saveValue(key, str, exp)
}

func (c *FileCache) SetDir(path string) {
	c.Dir = path
}

func (c *FileCache) Gc() error {
	// 过期清理由 Get 惰性删除完成；如需主动回收可在此遍历 Dir
	return nil
}

func NewFileCache() *FileCache {
	return &FileCache{
		locks: make(map[string]*sync.Mutex),
		Dir:   os.TempDir(),
	}
}
