package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

type ProcessMonitorService struct{}

// applyOverrides 把单设备覆盖配置应用到父规则副本
func applyOverrides(r model.ProcessMonitorRule, ov map[string]interface{}) model.ProcessMonitorRule {
	if ov == nil {
		return r
	}
	if v, ok := ov["name"].(string); ok && v != "" {
		r.Name = v
	}
	if v, ok := ov["type"].(string); ok && (v == "process" || v == "port") {
		r.Type = v
	}
	if v, ok := ov["target"].(string); ok && v != "" {
		r.Target = v
	}
	if v, ok := ov["interval"].(float64); ok && v > 0 {
		r.Interval = int(v)
	}
	if v, ok := ov["down_threshold"].(float64); ok && v >= 0 {
		r.DownThreshold = int(v)
	}
	if v, ok := ov["alert_config_id"].(float64); ok && v >= 0 {
		r.AlertConfigId = uint(v)
	}
	if v, ok := ov["enabled"].(bool); ok {
		if v {
			r.Enabled = 1
		} else {
			r.Enabled = 0
		}
	} else if v, ok := ov["enabled"].(float64); ok {
		r.Enabled = int(v)
	}
	return r
}

// peerMatchesSource 判断某设备当前是否属于集合规则（device_group / ab_tags）的覆盖范围。
// 成员关系动态取自设备当前所属设备组 / 地址簿标签，避免设备从设备组移除后监控快照未更新而继续报警。
func (s *ProcessMonitorService) peerMatchesSource(peer *model.Peer, ab *model.AddressBook, rule *model.ProcessMonitorRule) bool {
	switch rule.SourceType {
	case "device_group":
		gid, err := strconv.ParseUint(rule.SourceId, 10, 64)
		if err != nil {
			return false
		}
		return peer.RowId > 0 && peer.GroupId == uint(gid)
	case "ab_tags":
		ruleTags := splitTagSet(rule.SourceId)
		if len(ruleTags) == 0 {
			return false
		}
		if ab == nil || ab.RowId == 0 {
			return false
		}
		var peerTags []string
		if len(ab.Tags) > 0 {
			_ = json.Unmarshal(ab.Tags, &peerTags)
		}
		for _, t := range peerTags {
			if _, ok := ruleTags[t]; ok {
				return true
			}
		}
		return false
	}
	return false
}

// splitTagSet 将逗号分隔的标签串解析为集合
func splitTagSet(s string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			set[t] = struct{}{}
		}
	}
	return set
}

// loadOverride 读取某设备在某集合规则上的覆盖配置（不存在则返回 nil）
func (s *ProcessMonitorService) loadOverride(ruleId uint, peerId string) map[string]interface{} {
	var rp model.ProcessMonitorRulePeer
	DB.Where("rule_id = ? AND peer_id = ?", ruleId, peerId).First(&rp)
	if rp.RowId == 0 || len(rp.Overrides) == 0 {
		return nil
	}
	var ov map[string]interface{}
	if err := json.Unmarshal(rp.Overrides, &ov); err != nil {
		return nil
	}
	return ov
}

// findMatchingRule 查找匹配 peer+type+target 的规则（单设备优先，其次集合规则）
// 集合规则（device_group / ab_tags）的成员关系动态解析：设备从设备组/标签移除后不再匹配，停止报警。
func (s *ProcessMonitorService) findMatchingRule(peerId, typ, target string) *model.ProcessMonitorRule {
	// 1. 单设备规则
	var single model.ProcessMonitorRule
	DB.Where("peer_id = ? AND type = ? AND target = ? AND enabled = ?", peerId, typ, target, 1).
		Where("source_type = ? OR source_type = ?", "peers", "").
		First(&single)
	if single.RowId > 0 {
		return &single
	}

	// 2. 集合规则：动态解析成员关系（不再依赖 ProcessMonitorRulePeer 快照）
	peer := &model.Peer{}
	DB.Where("id = ?", peerId).First(peer)
	if peer.RowId == 0 {
		return &model.ProcessMonitorRule{}
	}
	ab := &model.AddressBook{}
	DB.Where("id = ?", peerId).First(ab)

	var groupRules []model.ProcessMonitorRule
	DB.Where("user_id = ? AND type = ? AND target = ? AND enabled = ? AND (source_type = ? OR source_type = ?)",
		peer.UserId, typ, target, 1, "device_group", "ab_tags").Find(&groupRules)
	for i := range groupRules {
		r := &groupRules[i]
		if !s.peerMatchesSource(peer, ab, r) {
			continue
		}
		rr := applyOverrides(*r, s.loadOverride(r.RowId, peerId))
		if rr.Enabled != 1 {
			// 该设备被单独排除（覆盖配置关闭）
			continue
		}
		return &rr
	}
	return &model.ProcessMonitorRule{}
}

// RulesByPeer 返回某设备启用的监控规则（含单设备规则与集合规则展开后的结果）
// 集合规则成员关系动态解析，设备从设备组/标签移除后不再下发对应监控配置。
func (s *ProcessMonitorService) RulesByPeer(peerId string) []model.ProcessMonitorRule {
	var rules []model.ProcessMonitorRule
	DB.Where("peer_id = ? AND enabled = ? AND (source_type = ? OR source_type = ?)", peerId, 1, "peers", "").
		Find(&rules)

	peer := &model.Peer{}
	DB.Where("id = ?", peerId).First(peer)
	if peer.RowId == 0 {
		return rules
	}
	ab := &model.AddressBook{}
	DB.Where("id = ?", peerId).First(ab)

	var groupRules []model.ProcessMonitorRule
	DB.Where("user_id = ? AND enabled = ? AND (source_type = ? OR source_type = ?)", peer.UserId, 1, "device_group", "ab_tags").
		Find(&groupRules)
	for i := range groupRules {
		r := &groupRules[i]
		if !s.peerMatchesSource(peer, ab, r) {
			continue
		}
		rr := applyOverrides(*r, s.loadOverride(r.RowId, peerId))
		if rr.Enabled != 1 {
			continue
		}
		rules = append(rules, rr)
	}
	return rules
}

// UpsertAndCheck 写入上报状态，并按规则判定是否触发告警
func (s *ProcessMonitorService) UpsertAndCheck(peerId, name, typ, target string, running bool, now int64) {
	runningInt := 0
	if running {
		runningInt = 1
	}
	rule := s.findMatchingRule(peerId, typ, target)

	st := &model.ProcessMonitorStatus{}
	DB.Where("peer_id = ? AND type = ? AND target = ?", peerId, typ, target).First(st)
	if st.RowId == 0 {
		st = &model.ProcessMonitorStatus{PeerId: peerId, Type: typ, Target: target}
	}
	st.Name = name
	st.Type = typ
	st.Target = target
	st.Running = runningInt
	st.RuleId = rule.RowId
	if runningInt == 1 {
		st.LastSeen = now
		st.DownSince = 0
		st.Alerted = 0
	} else {
		if st.DownSince == 0 {
			st.DownSince = now
		}
	}
	DB.Save(st)

	// 触发告警：down 持续超过阈值且尚未发送过告警
	if runningInt == 0 && rule.RowId > 0 && rule.AlertConfigId > 0 && rule.Enabled == 1 {
		if now-st.DownSince >= int64(rule.DownThreshold) && st.Alerted == 0 {
			s.fireAlert(rule, st)
			DB.Model(&model.ProcessMonitorStatus{}).Where("row_id = ?", st.RowId).Update("alerted", 1)
		}
	}
}

// fireAlert 复用既有告警通道发送通知
func (s *ProcessMonitorService) fireAlert(rule *model.ProcessMonitorRule, st *model.ProcessMonitorStatus) {
	cfg := &model.AlertConfig{}
	DB.Where("row_id = ?", rule.AlertConfigId).First(cfg)
	if cfg.RowId == 0 {
		return
	}
	typName := "进程"
	if rule.Type == "port" {
		typName = "端口"
	}
	title := fmt.Sprintf("进程监控告警：%s", rule.Name)
	content := fmt.Sprintf("设备：%s\n监控项：%s\n类型：%s\n目标：%s\n状态：未运行\n起始时间：%s",
		st.PeerId, rule.Name, typName, rule.Target,
		time.Unix(st.DownSince, 0).Format("2006-01-02 15:04:05"))
	AllService.NotifyService.SendByConfig(cfg, title, content)
}
