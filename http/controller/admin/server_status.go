package admin

import (
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type ServerStatus struct {
}

// Status 探测当前用户创建的所有服务器条目连通性，并返回 hbbr 负载(仅同机可用)
// @Tags 系统
// @Summary 服务器状态
// @Description 对用户在页面中添加的服务器条目做 TCP 探测，返回连通性/延迟；同时尝试查询 hbbr 负载(可选)
// @Produce  json
// @Success 200 {object} response.Response
// @Router /server_status [get]
// @Security token
func (ct *ServerStatus) Status(c *gin.Context) {
	results := service.AllService.ServerStatusService.ProbeAll()
	response.Success(c, gin.H{
		"list":       results,
		"hbbr_stats": hbbrStats(),
	})
}

// List 列出所有管理员共享的服务器探测条目
func (ct *ServerStatus) List(c *gin.Context) {
	list := service.AllService.ServerStatusService.ListAll()
	response.Success(c, gin.H{"list": list})
}

// Create 新建服务器探测条目（管理员共享）
func (ct *ServerStatus) Create(c *gin.Context) {
	f := &model.ServerStatusMonitor{}
	if err := c.ShouldBindJSON(f); err != nil || f.Host == "" || f.Name == "" {
		response.Fail(c, 101, "参数错误：名称与主机地址必填")
		return
	}
	if f.Protocol != "tcp" {
		f.Protocol = "tcp"
	}
	if f.Port < 0 || f.Port > 65535 {
		f.Port = 0
	}
	if f.Enabled != 0 {
		f.Enabled = 1
	}
	f.UserId = 0 // 0 表示管理员共享
	if err := service.AllService.ServerStatusService.Create(f); err != nil {
		response.Fail(c, 500, "保存失败："+err.Error())
		return
	}
	response.Success(c, f)
}

// Update 更新服务器探测条目（管理员共享）
func (ct *ServerStatus) Update(c *gin.Context) {
	f := &model.ServerStatusMonitor{}
	if err := c.ShouldBindJSON(f); err != nil || f.RowId == 0 {
		response.Fail(c, 101, "参数错误")
		return
	}
	if f.Protocol != "tcp" {
		f.Protocol = "tcp"
	}
	if f.Port < 0 || f.Port > 65535 {
		f.Port = 0
	}
	if f.Enabled != 0 {
		f.Enabled = 1
	}
	if err := service.AllService.ServerStatusService.Update(f); err != nil {
		response.Fail(c, 500, "保存失败："+err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 删除服务器探测条目（管理员共享）
func (ct *ServerStatus) Delete(c *gin.Context) {
	form := &struct {
		Id uint `json:"id"`
	}{}
	if err := c.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	service.AllService.ServerStatusService.Delete(form.Id)
	response.Success(c, nil)
}

// usageEntry 单条 hbbr 中继连接统计
type usageEntry struct {
	IP         string  `json:"ip"`
	Seconds    int64   `json:"seconds"`
	TrafficMB  float64 `json:"traffic_mb"`
	HighestKbps int64  `json:"highest_kbps"`
	AvgKbps    int64   `json:"avg_kbps"`
	SpeedKbps  int64   `json:"speed_kbps"`
}

// usageRe 匹配 hbbr `usage` 命令输出的每一行
var usageRe = regexp.MustCompile(`: (\d+)s ([\d.]+)MB (\d+)kb/s (\d+)kb/s (\d+)kb/s$`)

// hbbrStats 连接 hbbr 回环命令端口，发送 `u`(usage) 命令，解析中继连接数与负载。
// 注意：hbbr 仅接受来自本机(loopback)的命令连接，因此 api-server 必须与 hbbr 部署在同一主机。
func hbbrStats() gin.H {
	host := global.Config.Admin.RelayStatsHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := global.Config.Admin.RelayServerPort
	if port == 0 {
		port = 21117
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return gin.H{
			"available": false,
			"host":      addr,
			"error":     err.Error(),
		}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("u\n")); err != nil {
		return gin.H{
			"available": false,
			"host":      addr,
			"error":     err.Error(),
		}
	}
	buf := make([]byte, 65536)
	var sb strings.Builder
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	entries := parseUsage(sb.String())
	var totalMB float64
	var curSpeed, highestSpeed, connCount int64
	for _, e := range entries {
		totalMB += e.TrafficMB
		curSpeed += e.SpeedKbps
		if e.HighestKbps > highestSpeed {
			highestSpeed = e.HighestKbps
		}
		connCount++
	}
	return gin.H{
		"available":          true,
		"host":               addr,
		"connection_count":   connCount,
		"total_traffic_mb":   totalMB,
		"current_speed_kbps": curSpeed,
		"highest_speed_kbps": highestSpeed,
		"connections":        entries,
	}
}

// parseUsage 解析 hbbr usage 命令输出文本
func parseUsage(out string) []usageEntry {
	var entries []usageEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ": ")
		if idx < 0 {
			continue
		}
		ip := line[:idx]
		rest := line[idx+2:]
		m := usageRe.FindStringSubmatch(rest)
		if m == nil {
			continue
		}
		secs, _ := strconv.ParseInt(m[1], 10, 64)
		mb, _ := strconv.ParseFloat(m[2], 64)
		highest, _ := strconv.ParseInt(m[3], 10, 64)
		avg, _ := strconv.ParseInt(m[4], 10, 64)
		speed, _ := strconv.ParseInt(m[5], 10, 64)
		entries = append(entries, usageEntry{
			IP:          ip,
			Seconds:     secs,
			TrafficMB:   mb,
			HighestKbps: highest,
			AvgKbps:     avg,
			SpeedKbps:   speed,
		})
	}
	return entries
}
