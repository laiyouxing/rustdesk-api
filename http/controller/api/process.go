package api

import (
	"time"

	"github.com/gin-gonic/gin"
	requstform "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Process struct{}

// ProcessStatus 客户端上报监控状态（需 Bearer 鉴权，见 RustAuth 中间件）
// @Tags 进程监控
// @Summary 上报进程/端口监控状态
// @Description 客户端定时上报各监控项是否运行；响应回带该设备最新监控配置（后台集中下发）
// @Accept  json
// @Produce  json
// @Param body body api.ProcessStatusForm true "上报状态"
// @Success 200 {object} response.Response
// @Router /process/status [post]
// @Security token
func (p *Process) ProcessStatus(c *gin.Context) {
	f := &requstform.ProcessStatusForm{}
	if err := c.ShouldBindJSON(f); err != nil || f.PeerId == "" {
		response.Fail(c, 101, "参数错误")
		return
	}
	now := time.Now().Unix()
	for _, it := range f.Items {
		if it.Target == "" || (it.Type != "process" && it.Type != "port") {
			continue
		}
		service.AllService.ProcessMonitorService.UpsertAndCheck(f.PeerId, it.Name, it.Type, it.Target, it.Running, now)
	}
	response.Success(c, gin.H{"rules": toRuleOut(service.AllService.ProcessMonitorService.RulesByPeer(f.PeerId))})
}

// ProcessConfig 客户端拉取自己的监控配置（需 Bearer 鉴权）
// @Tags 进程监控
// @Summary 获取设备监控配置
// @Produce  json
// @Param peer_id query string true "设备 peer id"
// @Success 200 {object} response.Response
// @Router /process/config [get]
// @Security token
func (p *Process) ProcessConfig(c *gin.Context) {
	peerId := c.Query("peer_id")
	if peerId == "" {
		response.Fail(c, 101, "peer_id 不能为空")
		return
	}
	response.Success(c, gin.H{"rules": toRuleOut(service.AllService.ProcessMonitorService.RulesByPeer(peerId))})
}

// toRuleOut 转换为下发给客户端的精简结构
func toRuleOut(rules []model.ProcessMonitorRule) []gin.H {
	out := make([]gin.H, 0, len(rules))
	for _, r := range rules {
		out = append(out, gin.H{
			"name":    r.Name,
			"type":    r.Type,
			"target":  r.Target,
			"interval": r.Interval,
		})
	}
	return out
}
