package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type ProcessMonitor struct{}

// RuleList 列出当前用户配置的监控规则
func (c *ProcessMonitor) RuleList(ctx *gin.Context) {
	u := service.AllService.UserService.CurUser(ctx)
	var rules []model.ProcessMonitorRule
	service.DB.Where("user_id = ?", u.Id).Find(&rules)
	response.Success(ctx, gin.H{"list": rules})
}

// RuleCreate 新建监控规则
func (c *ProcessMonitor) RuleCreate(ctx *gin.Context) {
	f := &model.ProcessMonitorRule{}
	if err := ctx.ShouldBindJSON(f); err != nil || f.PeerId == "" || f.Target == "" {
		response.Fail(ctx, 101, "参数错误：peer_id 与 target 必填")
		return
	}
	if f.Type != "process" && f.Type != "port" {
		f.Type = "process"
	}
	if f.Interval <= 0 {
		f.Interval = 30
	}
	if f.DownThreshold <= 0 {
		f.DownThreshold = 300
	}
	u := service.AllService.UserService.CurUser(ctx)
	f.UserId = u.Id
	if err := service.DB.Create(f).Error; err != nil {
		response.Fail(ctx, 500, "保存失败："+err.Error())
		return
	}
	response.Success(ctx, f)
}

// RuleUpdate 更新监控规则
func (c *ProcessMonitor) RuleUpdate(ctx *gin.Context) {
	f := &model.ProcessMonitorRule{}
	if err := ctx.ShouldBindJSON(f); err != nil || f.RowId == 0 {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	u := service.AllService.UserService.CurUser(ctx)
	updates := map[string]interface{}{
		"name":           f.Name,
		"type":           f.Type,
		"target":         f.Target,
		"interval":       f.Interval,
		"down_threshold": f.DownThreshold,
		"alert_config_id": f.AlertConfigId,
		"enabled":        f.Enabled,
	}
	if err := service.DB.Model(&model.ProcessMonitorRule{}).Where("row_id = ? AND user_id = ?", f.RowId, u.Id).Updates(updates).Error; err != nil {
		response.Fail(ctx, 500, "保存失败："+err.Error())
		return
	}
	response.Success(ctx, nil)
}

// RuleDelete 删除监控规则（同时清理状态）
func (c *ProcessMonitor) RuleDelete(ctx *gin.Context) {
	form := &struct {
		Id uint `json:"id"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(ctx, 101, "ID不能为空")
		return
	}
	u := service.AllService.UserService.CurUser(ctx)
	service.DB.Where("row_id = ? AND user_id = ?", form.Id, u.Id).Delete(&model.ProcessMonitorRule{})
	service.DB.Where("rule_id = ?", form.Id).Delete(&model.ProcessMonitorStatus{})
	response.Success(ctx, nil)
}

// StatusList 查看各设备监控项实时状态（可按 peer_id 过滤）
func (c *ProcessMonitor) StatusList(ctx *gin.Context) {
	peerId := ctx.Query("peer_id")
	var list []model.ProcessMonitorStatus
	q := service.DB.Model(&model.ProcessMonitorStatus{})
	if peerId != "" {
		q = q.Where("peer_id = ?", peerId)
	}
	q.Order("peer_id, type, target").Find(&list)
	response.Success(ctx, gin.H{"list": list})
}
