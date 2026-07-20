package admin

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
	"strconv"
)

type ProcessMonitor struct{}

type ruleOut struct {
	model.ProcessMonitorRule
	Peers []peerOut `json:"peers"`
}

type peerOut struct {
	RowId     uint                   `json:"row_id"`
	PeerId    string                 `json:"peer_id"`
	Overrides map[string]interface{} `json:"overrides"`
}

// RuleList 列出当前用户配置的监控规则，集合规则附带关联的设备与覆盖配置
func (c *ProcessMonitor) RuleList(ctx *gin.Context) {
	u := service.AllService.UserService.CurUser(ctx)
	var rules []model.ProcessMonitorRule
	service.DB.Where("user_id = ?", u.Id).Order("source_type, created_at desc").Find(&rules)

	out := make([]ruleOut, 0, len(rules))
	for _, r := range rules {
		ro := ruleOut{ProcessMonitorRule: r, Peers: []peerOut{}}
		if r.SourceType == "device_group" || r.SourceType == "ab_tags" {
			// 成员关系动态解析：直接反映设备当前所属设备组 / 地址簿标签，
			// 不再依赖创建时写入 ProcessMonitorRulePeer 的静态快照
			var gid uint
			var tags []string
			if r.SourceType == "device_group" {
				if g, err := strconv.ParseUint(r.SourceId, 10, 64); err == nil {
					gid = uint(g)
				}
			} else {
				tags = strings.Split(r.SourceId, ",")
			}
			peerIds := c.resolvePeerIds(ctx, r.SourceType, nil, gid, tags)
			for _, pid := range peerIds {
				var rp model.ProcessMonitorRulePeer
				service.DB.Where("rule_id = ? AND peer_id = ?", r.RowId, pid).First(&rp)
				ov := map[string]interface{}{}
				if rp.RowId > 0 && len(rp.Overrides) > 0 {
					_ = json.Unmarshal(rp.Overrides, &ov)
				}
				ro.Peers = append(ro.Peers, peerOut{RowId: rp.RowId, PeerId: pid, Overrides: ov})
			}
		}
		out = append(out, ro)
	}
	response.Success(ctx, gin.H{"list": out})
}

// RuleCreate 新建单设备监控规则（手动输入模式）
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
	f.SourceType = "peers"
	f.SourceId = f.PeerId
	if err := service.DB.Create(f).Error; err != nil {
		response.Fail(ctx, 500, "保存失败："+err.Error())
		return
	}
	response.Success(ctx, f)
}

// RuleUpdate 更新监控规则（父规则字段 + 集合规则的子设备覆盖配置）
func (c *ProcessMonitor) RuleUpdate(ctx *gin.Context) {
	f := &struct {
		RowId         uint      `json:"row_id"`
		Name          string    `json:"name"`
		Type          string    `json:"type"`
		Target        string    `json:"target"`
		Interval      int       `json:"interval"`
		DownThreshold int       `json:"down_threshold"`
		AlertConfigId uint      `json:"alert_config_id"`
		Enabled       int       `json:"enabled"`
		Peers         []peerOut `json:"peers"`
	}{}
	if err := ctx.ShouldBindJSON(f); err != nil || f.RowId == 0 {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	u := service.AllService.UserService.CurUser(ctx)
	var existing model.ProcessMonitorRule
	service.DB.Where("row_id = ? AND user_id = ?", f.RowId, u.Id).First(&existing)
	if existing.RowId == 0 {
		response.Fail(ctx, 101, "规则不存在")
		return
	}

	err := service.DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"name":            f.Name,
			"type":            f.Type,
			"target":          f.Target,
			"interval":        f.Interval,
			"down_threshold":  f.DownThreshold,
			"alert_config_id": f.AlertConfigId,
			"enabled":         f.Enabled,
		}
		if err := tx.Model(&model.ProcessMonitorRule{}).Where("row_id = ?", f.RowId).Updates(updates).Error; err != nil {
			return err
		}
		if existing.SourceType == "device_group" || existing.SourceType == "ab_tags" {
			if err := tx.Where("rule_id = ?", f.RowId).Delete(&model.ProcessMonitorRulePeer{}).Error; err != nil {
				return err
			}
			for _, p := range f.Peers {
				if p.PeerId == "" {
					continue
				}
			ov := custom_types.AutoJson([]byte("{}"))
			if p.Overrides != nil {
				b, _ := json.Marshal(p.Overrides)
				ov = custom_types.AutoJson(b)
			}
			rp := &model.ProcessMonitorRulePeer{RuleId: f.RowId, PeerId: p.PeerId, Overrides: ov}
				if err := tx.Create(rp).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		response.Fail(ctx, 500, "保存失败："+err.Error())
		return
	}
	response.Success(ctx, nil)
}

// RuleDelete 删除监控规则（同时清理状态与集合规则子表）
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
	service.DB.Where("rule_id = ?", form.Id).Delete(&model.ProcessMonitorRulePeer{})
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
func (c *ProcessMonitor) PeerSources(ctx *gin.Context) {
	u := service.AllService.UserService.CurUser(ctx)

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

// resolveSourceName 根据来源类型与ID解析展示名称
func (c *ProcessMonitor) resolveSourceName(sourceType string, sourceId string, groupId uint, tags []string) string {
	switch sourceType {
	case "device_group":
		var g model.DeviceGroup
		service.DB.Where("id = ?", groupId).First(&g)
		if g.Id > 0 {
			return g.Name
		}
	case "ab_tags":
		return sourceId
	}
	return ""
}

// RuleBatchCreate 按设备组 / 地址簿标签创建集合规则（一条规则对应一个集合）
func (c *ProcessMonitor) RuleBatchCreate(ctx *gin.Context) {
	form := &struct {
		SourceType    string   `json:"source_type"` // device_group | ab_tags
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
	if form.SourceType != "device_group" && form.SourceType != "ab_tags" {
		response.Fail(ctx, 101, "参数错误：source_type 仅支持 device_group / ab_tags")
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

	u := service.AllService.UserService.CurUser(ctx)
	var sourceId string
	if form.SourceType == "device_group" {
		if form.GroupId == 0 {
			response.Fail(ctx, 101, "请选择设备组")
			return
		}
		sourceId = fmt.Sprintf("%d", form.GroupId)
	} else {
		if len(form.Tags) == 0 {
			response.Fail(ctx, 101, "请选择地址簿标签")
			return
		}
		// 标签集合排序后拼接，保证同一批标签的 source_id 一致
		sort.Strings(form.Tags)
		sourceId = strings.Join(form.Tags, ",")
	}

	peerIds := c.resolvePeerIds(ctx, form.SourceType, nil, form.GroupId, form.Tags)
	if len(peerIds) == 0 {
		response.Fail(ctx, 101, "未匹配到任何设备")
		return
	}

	var rule model.ProcessMonitorRule
	service.DB.Where("user_id = ? AND source_type = ? AND source_id = ? AND type = ? AND target = ?",
		u.Id, form.SourceType, sourceId, form.Type, form.Target).First(&rule)

	if rule.RowId > 0 {
		// 已存在同集合规则，更新父规则并追加新设备
		created, skipped := 0, 0
		err := service.DB.Transaction(func(tx *gorm.DB) error {
			updates := map[string]interface{}{
				"name":            form.Name,
				"interval":        form.Interval,
				"down_threshold":  form.DownThreshold,
				"alert_config_id": form.AlertConfigId,
				"enabled":         form.Enabled,
			}
			if err := tx.Model(&model.ProcessMonitorRule{}).Where("row_id = ?", rule.RowId).Updates(updates).Error; err != nil {
				return err
			}
			var existing []string
			tx.Model(&model.ProcessMonitorRulePeer{}).Where("rule_id = ?", rule.RowId).Pluck("peer_id", &existing)
			existingSet := make(map[string]struct{}, len(existing))
			for _, id := range existing {
				existingSet[id] = struct{}{}
			}
			for _, pid := range peerIds {
				if _, ok := existingSet[pid]; ok {
					skipped++
					continue
				}
			rp := &model.ProcessMonitorRulePeer{RuleId: rule.RowId, PeerId: pid, Overrides: custom_types.AutoJson([]byte("{}"))}
			if err := tx.Create(rp).Error; err != nil {
				return err
			}
			created++
			}
			return nil
		})
		if err != nil {
			response.Fail(ctx, 500, "保存失败："+err.Error())
			return
		}
		response.Success(ctx, gin.H{"created": created, "skipped": skipped, "matched": len(peerIds)})
		return
	}

	// 新建集合规则
	rule = model.ProcessMonitorRule{
		UserId:        u.Id,
		SourceType:    form.SourceType,
		SourceId:      sourceId,
		SourceName:    c.resolveSourceName(form.SourceType, sourceId, form.GroupId, form.Tags),
		Name:          form.Name,
		Type:          form.Type,
		Target:        form.Target,
		Interval:      form.Interval,
		DownThreshold: form.DownThreshold,
		AlertConfigId: form.AlertConfigId,
		Enabled:       form.Enabled,
	}
	err := service.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
		for _, pid := range peerIds {
			rp := &model.ProcessMonitorRulePeer{RuleId: rule.RowId, PeerId: pid, Overrides: custom_types.AutoJson([]byte("{}"))}
			if err := tx.Create(rp).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		response.Fail(ctx, 500, "保存失败："+err.Error())
		return
	}
	response.Success(ctx, gin.H{"created": len(peerIds), "skipped": 0, "matched": len(peerIds)})
}
