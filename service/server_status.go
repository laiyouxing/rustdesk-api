package service

import (
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

type ServerStatusService struct{}

// ListAll 返回所有管理员共享的服务器探测条目
func (s *ServerStatusService) ListAll() []model.ServerStatusMonitor {
	var list []model.ServerStatusMonitor
	DB.Where("enabled = ?", 1).Order("row_id").Find(&list)
	return list
}

func (s *ServerStatusService) Create(m *model.ServerStatusMonitor) error {
	return DB.Create(m).Error
}

func (s *ServerStatusService) Update(m *model.ServerStatusMonitor) error {
	updates := map[string]interface{}{
		"name":     m.Name,
		"host":     m.Host,
		"port":     m.Port,
		"protocol": m.Protocol,
		"enabled":  m.Enabled,
	}
	return DB.Model(&model.ServerStatusMonitor{}).Where("row_id = ?", m.RowId).Updates(updates).Error
}

func (s *ServerStatusService) Delete(id uint) error {
	DB.Where("row_id = ?", id).Delete(&model.ServerStatusMonitor{})
	return nil
}

// ProbeResult 单条探测结果
type ProbeResult struct {
	RowId     uint   `json:"row_id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Addr      string `json:"addr"`
	Status    string `json:"status"` // up | down
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error"`
}

// Probe 对单条条目做 TCP 连通性探测
func (s *ServerStatusService) Probe(m model.ServerStatusMonitor) ProbeResult {
	addr := m.Host
	if m.Port > 0 {
		addr = net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
	}
	res := ProbeResult{RowId: m.RowId, Name: m.Name, Host: m.Host, Port: m.Port, Addr: addr}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		res.Status = "down"
		res.Error = err.Error()
		return res
	}
	_ = conn.Close()
	res.Status = "up"
	res.LatencyMs = time.Since(start).Milliseconds()
	return res
}

// ProbeAll 探测所有启用中的条目（管理员共享，并发探测）
// 并发探测避免条目过多时串行耗时线性叠加（每条最长 3s），
// 防止总响应时间超过服务端 write-timeout 导致连接被强制断开。
func (s *ServerStatusService) ProbeAll() []ProbeResult {
	list := s.ListAll()
	results := make([]ProbeResult, len(list))
	var wg sync.WaitGroup
	for i := range list {
		wg.Add(1)
		go func(idx int, m model.ServerStatusMonitor) {
			defer wg.Done()
			results[idx] = s.Probe(m)
		}(i, list[i])
	}
	wg.Wait()
	return results
}
