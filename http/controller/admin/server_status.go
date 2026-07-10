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
)

type ServerStatus struct {
}

// Status 检查 hbbs/hbbr 端口连通性并查询 hbbr 负载/连接数，仅管理员可访问
// @Tags 系统
// @Summary 服务器状态
// @Description 通过 TCP 探测 hbbs(rendezvous) 与 hbbr(relay) 服务端口返回连通性与延迟；
// 同时经 hbbr 回环命令接口(usage)查询中继连接数与负载(仅 api-server 与 hbbr 同机时可用)
// @Produce  json
// @Success 200 {object} response.Response
// @Router /server_status [get]
// @Security token
func (ct *ServerStatus) Status(c *gin.Context) {
	hbbs := probe(global.Config.Rustdesk.IdServer, global.Config.Admin.IdServerPort)
	hbbr := probe(global.Config.Rustdesk.RelayServer, global.Config.Admin.RelayServerPort)
	response.Success(c, gin.H{
		"hbbs":       hbbs,
		"hbbr":       hbbr,
		"hbbr_stats": hbbrStats(),
	})
}

// probe 通过 TCP 探测指定地址，返回状态/延迟/错误信息
func probe(host string, defaultPort int) gin.H {
	if host == "" {
		return gin.H{
			"host":       "",
			"status":     "not_configured",
			"latency_ms": 0,
			"error":      "",
		}
	}
	addr := host
	if !strings.Contains(host, ":") {
		addr = net.JoinHostPort(host, strconv.Itoa(defaultPort))
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return gin.H{
			"host":       addr,
			"status":     "down",
			"latency_ms": 0,
			"error":      err.Error(),
		}
	}
	defer conn.Close()
	return gin.H{
		"host":       addr,
		"status":     "up",
		"latency_ms": time.Since(start).Milliseconds(),
		"error":      "",
	}
}

// usageEntry 单条 hbbr 中继连接统计
type usageEntry struct {
	IP          string  `json:"ip"`
	Seconds      int64   `json:"seconds"`
	TrafficMB    float64 `json:"traffic_mb"`
	HighestKbps  int64   `json:"highest_kbps"`
	AvgKbps      int64   `json:"avg_kbps"`
	SpeedKbps    int64   `json:"speed_kbps"`
}

// usageRe 匹配 hbbr `usage` 命令输出的每一行：
// {ip}: {seconds}s {MB}MB {highest}kb/s {avg}kb/s {speed}kb/s
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
			IP:         ip,
			Seconds:    secs,
			TrafficMB:  mb,
			HighestKbps: highest,
			AvgKbps:    avg,
			SpeedKbps:  speed,
		})
	}
	return entries
}
