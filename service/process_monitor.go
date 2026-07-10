package service

import (
	"encoding/json"
	"fmt"
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

// findMatchingRule 查找匹配 peer+type+target 的规则（单设备优先，其次集合规则）
func (s *ProcessMonitorService) findMatchingRule(peerId, typ, target string) *model.ProcessMonitorRule {
	var single model.ProcessMonitorRule
	DB.Where("peer_id = ? AND type = ? AND target = ? AND enabled = ?", peerId, typ, target, 1).
		Where("source_type = ? OR source_type = ?", "peers", "").
		First(&single)
	if single.RowId > 0 {
		return &single
	}

	var peers []model.ProcessMonitorRulePeer
	DB.Where("peer_id = ?", peerId).Find(&peers)
	if len(peers) == 0 {
		return &model.ProcessMonitorRule{}
	}
	ids := make([]uint, 0, len(peers))
	for _, p := range peers {
		ids = append(ids, p.RuleId)
	}
	var groupRules []model.ProcessMonitorRule
	DB.Where("row_id IN ? AND type = ? AND target = ? AND enabled = ?", ids, typ, target, 1).Find(&groupRules)
	if len(groupRules) == 0 {
		return &model.ProcessMonitorRule{}
	}
	r := groupRules[0]
	var overrides map[string]interface{}
	for _, p := range peers {
		if p.RuleId == r.RowId && len(p.Overrides) > 0 {
			_ = json.Unmarshal(p.Overrides, &overrides)
			break
		}
	}
	rr := applyOverrides(r, overrides)
	return &rr
}

// RulesByPeer 返回某设备启用的监控规则（含单设备规则与集合规则展开后的结果）
func (s *ProcessMonitorService) RulesByPeer(peerId string) []model.ProcessMonitorRule {
	var rules []model.ProcessMonitorRule
	DB.Where("peer_id = ? AND enabled = ? AND (source_type = ? OR source_type = ?)", peerId, 1, "peers", "").
		Find(&rules)

	var peers []model.ProcessMonitorRulePeer
	DB.Where("peer_id = ?", peerId).Find(&peers)
	if len(peers) > 0 {
		ids := make([]uint, 0, len(peers))
		for _, p := range peers {
			ids = append(ids, p.RuleId)
		}
		var groupRules []model.ProcessMonitorRule
		DB.Where("row_id IN ? AND enabled = ?", ids, 1).Find(&groupRules)
		for _, gr := range groupRules {
			var overrides map[string]interface{}
			for _, p := range peers {
				if p.RuleId == gr.RowId && len(p.Overrides) > 0 {
					_ = json.Unmarshal(p.Overrides, &overrides)
					break
				}
			}
			rules = append(rules, applyOverrides(gr, overrides))
		}
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
