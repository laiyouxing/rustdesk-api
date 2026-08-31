package service

import (
	"encoding/json"
	"fmt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"time"
)

// peerNotifyRecord 每个 peer 的通知记录（用于 NotifiedPeers JSON 字段）
type peerNotifyRecord struct {
	Count     int   `json:"c"` // 当天通知次数
	LastTime  int64 `json:"t"` // 最后通知时间戳
	Weight    int   `json:"w"` // 离线权重：每 5 分钟检测一次，离线则 +1
	WeightDay int   `json:"d"` // 权重所属日期（YYYYMMDD），用于每日重置
}

// parseNotifiedPeers 兼容解析新旧两种格式的 NotifiedPeers
func parseNotifiedPeers(data string) map[string]*peerNotifyRecord {
	result := make(map[string]*peerNotifyRecord)
	if data == "" {
		return result
	}
	// 尝试新格式 map[string]*peerNotifyRecord
	m := make(map[string]*peerNotifyRecord)
	if err := json.Unmarshal([]byte(data), &m); err == nil {
		for k, v := range m {
			result[k] = v
		}
		return result
	}
	// 兼容旧格式 map[string]int64
	old := make(map[string]int64)
	if err := json.Unmarshal([]byte(data), &old); err == nil {
		for k, v := range old {
			result[k] = &peerNotifyRecord{Count: 1, LastTime: v}
		}
	}
	return result
}

// isSameDay 判断两个时间戳是否在同一天（本地时间）
func isSameDay(a, b int64) bool {
	ta := time.Unix(a, 0)
	tb := time.Unix(b, 0)
	return ta.Year() == tb.Year() && ta.YearDay() == tb.YearDay()
}

// dayKey 返回本地日期的整数表示（YYYYMMDD），用于每日重置权重与连续天数计算
func dayKey(t int64) int {
	tm := time.Unix(t, 0)
	return tm.Year()*10000 + int(tm.Month())*100 + tm.Day()
}

const (
	maxNotifyPerDay        = 3                // 同一设备每天最多通知次数
	notifyCooldown         = 600              // 单个 peer 最短通知间隔（秒），避免短时间内重复触发
	offlineWeightThreshold = 10               // 离线权重达到该值才触发告警（10 次 × 5 分钟 = 50 分钟）
	checkInterval          = 5 * time.Minute // 离线检测间隔
)

type AlertService struct{}

func (s *AlertService) StartChecker() {
	AllService.AlertService = s
	go func() {
		for {
			s.checkOfflineDevices()
			time.Sleep(checkInterval)
		}
	}()
	Logger.Info("Alert checker started")
}

// getMonitoredPeerIds 返回该告警配置应监控的设备ID列表
// MonitorAll=1: 监控该用户地址簿中所有设备
// MonitorAll=2: 仅监控 alert_targets 中选中的设备或集合
func (s *AlertService) getMonitoredPeerIds(cfg *model.AlertConfig) ([]string, bool) {
	if cfg.MonitorAll == 1 {
		// 用户自己的地址簿
		var abEntries []model.AddressBook

		var ownColls []model.AddressBookCollection
		DB.Where("user_id = ?", cfg.UserId).Find(&ownColls)
		ownCollIds := []uint{0}
		for _, col := range ownColls {
			ownCollIds = append(ownCollIds, col.Id)
		}

		// 他人分享给该用户的集合
		user := &model.User{}
		DB.First(user, cfg.UserId)
		if user.Id > 0 {
			var rules []model.AddressBookCollectionRule
			ruleQuery := DB.Where("type = ? AND to_id = ?",
				model.ShareAddressBookRuleTypePersonal, user.Id)
			if user.GroupId > 0 {
				ruleQuery = DB.Where(
					"(type = ? AND to_id = ?) OR (type = ? AND to_id = ?)",
					model.ShareAddressBookRuleTypePersonal, user.Id,
					model.ShareAddressBookRuleTypeGroup, user.GroupId,
				)
			}
			ruleQuery.Find(&rules)
			for _, rule := range rules {
				ownCollIds = append(ownCollIds, rule.CollectionId)
			}
		}

		DB.Where("collection_id in (?)", ownCollIds).Find(&abEntries)
		if len(abEntries) == 0 {
			return nil, true
		}
		var peerIds []string
		for _, ab := range abEntries {
			peerIds = append(peerIds, ab.Id)
		}
		return peerIds, false
	}

	var targets []model.AlertTarget
	DB.Where("alert_id = ?", cfg.RowId).Find(&targets)
	if len(targets) == 0 {
		return s.getMonitoredPeerIds(&model.AlertConfig{
			MonitorAll: 1,
			UserId:     cfg.UserId,
		})
	}

	var peerIds []string
	for _, t := range targets {
		if t.TargetType == "peer" {
			peerIds = append(peerIds, t.TargetId)
		} else if t.TargetType == "collection" {
			var abEntries []model.AddressBook
			DB.Where("collection_id = ?", t.TargetId).Find(&abEntries)
			for _, ab := range abEntries {
				peerIds = append(peerIds, ab.Id)
			}
		}
	}
	return peerIds, false
}

func (s *AlertService) checkOfflineDevices() {
	var configs []model.AlertConfig
	DB.Where("enabled = 1 AND user_id > 0").Find(&configs)
	if len(configs) == 0 {
		return
	}

	now := time.Now().Unix()
	today := dayKey(now)
	prevDay := dayKey(now - 86400)

	// 按用户分组处理：每个用户的站内消息配置（若存在）
	userStationCfg := make(map[uint]*model.AlertConfig)
	for i := range configs {
		if configs[i].Channel == "station" {
			userStationCfg[configs[i].UserId] = &configs[i]
		}
	}

	// ===== 第一步：解析所有规则监控范围，构建被监测设备池（去重） =====
	// 所有规则统一从设备池中取设备、一次性查询在线状态，避免同一设备被多个规则重复加载，减轻数据库负担。
	type ruleCfg struct {
		cfg        *model.AlertConfig
		activeDays int
		peerIds    []string
	}
	var ruleCfgs []ruleCfg
	pool := make(map[string]struct{})
	poolDays := 0 // 设备池活跃窗口取各规则 activeDays 的最大值
	for i := range configs {
		cfg := &configs[i]
		if cfg.Channel == "station" {
			continue
		}
		peerIds, monitorAll := s.getMonitoredPeerIds(cfg)
		if !monitorAll && len(peerIds) == 0 {
			continue
		}
		activeDays := cfg.ActiveDays
		if activeDays <= 0 {
			activeDays = 30
		}
		ruleCfgs = append(ruleCfgs, ruleCfg{cfg: cfg, activeDays: activeDays, peerIds: peerIds})
		for _, id := range peerIds {
			pool[id] = struct{}{}
		}
		if activeDays > poolDays {
			poolDays = activeDays
		}
	}
	if len(ruleCfgs) == 0 {
		return
	}

	// 一次性加载设备池的在线信息（排除超过 poolDays 天未上线的设备）
	poolIds := make([]string, 0, len(pool))
	for id := range pool {
		poolIds = append(poolIds, id)
	}
	peerMap := make(map[string]model.Peer, len(pool))
	var poolPeers []model.Peer
	DB.Select("id, last_online_time, hostname, alias").
		Where("last_online_time > ?", now-int64(poolDays)*86400).
		Where("id in (?)", poolIds).
		Find(&poolPeers)
	for _, p := range poolPeers {
		peerMap[p.Id] = p
	}

	// ===== 第二步：按规则从池中挑选设备，计算权重并推送 =====
	for _, r := range ruleCfgs {
		cfg := r.cfg
		threshold := int64(cfg.OfflineMin * 60)
		if threshold <= 0 {
			threshold = 300
		}

		// 从池中挑选该规则监控的设备（再按规则自身的活跃窗口过滤）
		var peers []model.Peer
		window := now - int64(r.activeDays)*86400
		for _, id := range r.peerIds {
			if p, ok := peerMap[id]; ok && p.LastOnlineTime > window {
				peers = append(peers, p)
			}
		}
		if len(peers) == 0 {
			continue
		}

		notifiedMap := parseNotifiedPeers(cfg.NotifiedPeers)

		// 重新上线检测：任意被监控设备近期上线，则重置“连续3天离线”限制
		for _, peer := range peers {
			if peer.LastOnlineTime > now-300 {
				if cfg.ConsecutiveTriggerDays != 0 || cfg.LastTriggerDay != 0 {
					cfg.ConsecutiveTriggerDays = 0
					cfg.LastTriggerDay = 0
				}
				break
			}
		}

		// 更新离线权重：每 5 分钟检测一次，离线则 +1；每日 / 上线重置
		type candidate struct {
			peer   model.Peer
			weight int
		}
		var candidates []candidate
		for _, peer := range peers {
			rec, ok := notifiedMap[peer.Id]
			if !ok {
				rec = &peerNotifyRecord{}
			}
			// 每日重置权重
			if rec.WeightDay != today {
				rec.Weight = 0
				rec.WeightDay = today
			}
			switch {
			case peer.LastOnlineTime > now-300:
				// 已上线：重置权重
				rec.Weight = 0
				rec.WeightDay = today
			case peer.LastOnlineTime < now-threshold:
				// 离线（超过阈值时长）：权重 +1
				// 如果设备在告警创建前就已离线，跳过本次离线事件（不追溯历史）
				if cfg.CreatedAt > 0 && peer.LastOnlineTime < cfg.CreatedAt {
					break
				}
				rec.Weight++
			default:
				// 刚离线但尚未超过阈值：不计入权重
				rec.Weight = 0
				rec.WeightDay = today
			}
			notifiedMap[peer.Id] = rec
			if rec.Weight >= offlineWeightThreshold {
				candidates = append(candidates, candidate{peer: peer, weight: rec.Weight})
			}
		}

		// 同一设备每天最多推送次数（规则可配，默认 3）
		maxPerDay := cfg.MaxNotifyPerDay
		if maxPerDay <= 0 {
			maxPerDay = maxNotifyPerDay
		}

		// 筛选可通知的设备（排除当天已达上限或冷却期内的）
		var alertPeers []candidate
		for _, c := range candidates {
			rec := notifiedMap[c.peer.Id]
			if isSameDay(rec.LastTime, now) && rec.Count >= maxPerDay {
				continue
			}
			if now-rec.LastTime < notifyCooldown {
				continue
			}
			alertPeers = append(alertPeers, c)
		}

		// 连续 3 天以上触发则不再推送邮件告警
		if cfg.ConsecutiveTriggerDays >= 3 {
			Logger.Infof("alert config %d: consecutive offline trigger days >= 3, skip email push", cfg.RowId)
			s.persistAlertCfg(cfg.RowId, notifiedMap, cfg.LastNotifiedAt, cfg.ConsecutiveTriggerDays, cfg.LastTriggerDay)
			continue
		}

		if len(alertPeers) == 0 {
			s.persistAlertCfg(cfg.RowId, notifiedMap, cfg.LastNotifiedAt, cfg.ConsecutiveTriggerDays, cfg.LastTriggerDay)
			continue
		}

		pushedAny := false
		for _, c := range alertPeers {
			peer := c.peer
			hostname := peer.Hostname
			if hostname == "" {
				hostname = peer.Id
			}
			alias := peer.Alias
			if alias == "" {
				alias = hostname
			}
			lastOnline := time.Unix(peer.LastOnlineTime, 0).Format("2006-01-02 15:04:05")
			title := "设备离线告警"
			offlineMinutes := (now - peer.LastOnlineTime) / 60
			content := fmt.Sprintf("设备：%s\n别名：%s\nID：%s\n离线时长：%d 分钟\n最后在线：%s\n离线权重：%d",
				hostname, alias, peer.Id, offlineMinutes, lastOnline, c.weight)

			// 发送外部渠道通知（邮件等）
			AllService.NotifyService.SendByConfig(cfg, title, content)

			// 该用户是否有站内消息配置？有则发站内消息
			if stationCfg, ok := userStationCfg[cfg.UserId]; ok && stationCfg != nil {
				AllService.NotifyService.SendStationMessage(cfg.UserId, title, content, peer.Id)
			}

			// 更新该 peer 的通知记录
			rec := notifiedMap[peer.Id]
			if isSameDay(rec.LastTime, now) {
				rec.Count++
				rec.LastTime = now
			} else {
				rec.Count = 1
				rec.LastTime = now
			}
			notifiedMap[peer.Id] = rec
			pushedAny = true
		}

		// 清理非今天的记录（以权重所属日期为准），避免 map 无限增长，
		// 同时保留当天仍在累积权重的离线设备记录
		for k, v := range notifiedMap {
			if v.WeightDay != today {
				delete(notifiedMap, k)
			}
		}

		// 更新连续触发天数（每个自然日只计一次）
		if pushedAny {
			if cfg.LastTriggerDay == today {
				// 当日已统计，不重复累计
			} else if cfg.LastTriggerDay == prevDay {
				cfg.ConsecutiveTriggerDays++
			} else {
				cfg.ConsecutiveTriggerDays = 1
			}
			cfg.LastTriggerDay = today
		}

		s.persistAlertCfg(cfg.RowId, notifiedMap, now, cfg.ConsecutiveTriggerDays, cfg.LastTriggerDay)
	}
}

// persistAlertCfg 持久化告警配置的运行状态（通知记录、连续触发天数等）
func (s *AlertService) persistAlertCfg(rowId uint, notifiedMap map[string]*peerNotifyRecord, lastNotifiedAt int64, consecutive int, lastTriggerDay int) {
	encoded, _ := json.Marshal(notifiedMap)
	DB.Model(&model.AlertConfig{}).Where("row_id = ?", rowId).Updates(map[string]interface{}{
		"last_notified_at":         lastNotifiedAt,
		"notified_peers":           string(encoded),
		"consecutive_trigger_days": consecutive,
		"last_trigger_day":         lastTriggerDay,
	})
}
