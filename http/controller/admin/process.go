package admin

import (
	"encoding/json"

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

// PeerSources 返回可用于批量选择设备的来源：设备组 + 地址簿标签
// 便于前端在“新增规则”时按设备组 / 地址簿标签批量配置
func (c *ProcessMonitor) PeerSources(ctx *gin.Context) {
	u := service.AllService.UserService.CurUser(ctx)

	// 设备组（全局，附带每组在线设备数）
	type groupItem struct {
		Id    uint   `json:"id"`
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	var groups []groupItem
	dgList := service.AllService.GroupService.DeviceGroupList(1, 999, nil)
	for _, g := range dgList.DeviceGroups {
		var cnt int64
		service.DB.Model(&model.Peer{}).Where("group_id = ?", g.Id).Count(&cnt)
		groups = append(groups, groupItem{Id: g.Id, Name: g.Name, Count: cnt})
	}
	if groups == nil {
		groups = []groupItem{}
	}

	// 地址簿标签（当前用户名下），统计每个标签的设备数
	var abs []model.AddressBook
	service.DB.Where("user_id = ?", u.Id).Find(&abs)
	tagCount := make(map[string]int)
	for _, ab := range abs {
		var tags []string
		if len(ab.Tags) == 0 {
			continue
		}
		if err := json.Unmarshal([]byte(ab.Tags), &tags); err != nil {
			continue
		}
		for _, t := range tags {
			if t == "" {
				continue
			}
			tagCount[t]++
		}
	}
	type tagItem struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	tagList := make([]tagItem, 0, len(tagCount))
	for t, cnt := range tagCount {
		tagList = append(tagList, tagItem{Tag: t, Count: cnt})
	}

	response.Success(ctx, gin.H{"device_groups": groups, "ab_tags": tagList})
}

// resolvePeerIds 根据来源解析目标设备ID集合（去重）
func (c *ProcessMonitor) resolvePeerIds(ctx *gin.Context, sourceType string, peerIds []string, groupId uint, tags []string) []string {
	set := make(map[string]struct{})
	add := func(id string) {
		if id != "" {
			set[id] = struct{}{}
		}
	}
	switch sourceType {
	case "device_group":
		if groupId > 0 {
			var ids []string
			service.DB.Model(&model.Peer{}).Where("group_id = ?", groupId).Pluck("id", &ids)
			for _, id := range ids {
				add(id)
			}
		}
	case "ab_tags":
		if len(tags) > 0 {
			u := service.AllService.UserService.CurUser(ctx)
			var abs []model.AddressBook
			service.DB.Where("user_id = ?", u.Id).Find(&abs)
			tagSet := make(map[string]struct{}, len(tags))
			for _, t := range tags {
				tagSet[t] = struct{}{}
			}
			for _, ab := range abs {
				var abTags []string
				if len(ab.Tags) == 0 {
					continue
				}
				if err := json.Unmarshal([]byte(ab.Tags), &abTags); err != nil {
					continue
				}
				for _, t := range abTags {
					if _, ok := tagSet[t]; ok {
						add(ab.Id)
						break
					}
				}
			}
		}
	default: // peers
		for _, id := range peerIds {
			add(id)
		}
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	return result
}

// RuleBatchCreate 按设备组 / 地址簿标签 / 手动选择的设备列表批量创建监控规则
func (c *ProcessMonitor) RuleBatchCreate(ctx *gin.Context) {
	form := &struct {
		SourceType    string   `json:"source_type"` // peers | device_group | ab_tags
		PeerIds       []string `json:"peer_ids"`
		GroupId       uint     `json:"group_id"`
		Tags          []string `json:"tags"`
		Name          string   `json:"name"`
		Type          string   `json:"type"`
		Target        string   `json:"target"`
		Interval      int      `json:"interval"`
		DownThreshold int      `json:"down_threshold"`
		AlertConfigId uint     `json:"alert_config_id"`
		Enabled       int      `json:"enabled"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil || form.Target == "" {
		response.Fail(ctx, 101, "参数错误：target 必填")
		return
	}
	if form.Type != "process" && form.Type != "port" {
		form.Type = "process"
	}
	if form.Interval <= 0 {
		form.Interval = 30
	}
	if form.DownThreshold <= 0 {
		form.DownThreshold = 300
	}

	peerIds := c.resolvePeerIds(ctx, form.SourceType, form.PeerIds, form.GroupId, form.Tags)
	if len(peerIds) == 0 {
		response.Fail(ctx, 101, "未匹配到任何设备")
		return
	}

	u := service.AllService.UserService.CurUser(ctx)
	created, skipped := 0, 0
	for _, pid := range peerIds {
		// 跳过同设备下相同 type+target 的重复规则
		var cnt int64
		service.DB.Model(&model.ProcessMonitorRule{}).
			Where("user_id = ? AND peer_id = ? AND type = ? AND target = ?", u.Id, pid, form.Type, form.Target).
			Count(&cnt)
		if cnt > 0 {
			skipped++
			continue
		}
		rule := &model.ProcessMonitorRule{
			UserId:        u.Id,
			PeerId:        pid,
			Name:          form.Name,
			Type:          form.Type,
			Target:        form.Target,
			Interval:      form.Interval,
			DownThreshold: form.DownThreshold,
			AlertConfigId: form.AlertConfigId,
			Enabled:       form.Enabled,
		}
		if err := service.DB.Create(rule).Error; err == nil {
			created++
		}
	}
	response.Success(ctx, gin.H{"created": created, "skipped": skipped, "matched": len(peerIds)})
}
