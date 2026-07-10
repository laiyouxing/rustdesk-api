package service

import (
	"fmt"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

type ProcessMonitorService struct{}

// RulesByPeer 返回某设备启用的监控规则（下发给客户端）
func (s *ProcessMonitorService) RulesByPeer(peerId string) []model.ProcessMonitorRule {
	var rules []model.ProcessMonitorRule
	DB.Where("peer_id = ? AND enabled = ?", peerId, 1).Find(&rules)
	return rules
}

// UpsertAndCheck 写入上报状态，并按规则判定是否触发告警
func (s *ProcessMonitorService) UpsertAndCheck(peerId, name, typ, target string, running bool, now int64) {
	runningInt := 0
	if running {
		runningInt = 1
	}
	rule := &model.ProcessMonitorRule{}
	DB.Where("peer_id = ? AND type = ? AND target = ? AND enabled = ?", peerId, typ, target, 1).First(rule)

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
	if runningInt == 0 && rule.RowId > 0 && rule.AlertConfigId > 0 {
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
