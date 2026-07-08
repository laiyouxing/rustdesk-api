package service

import (
	"encoding/json"
	"fmt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"time"
)

// peerNotifyRecord 每个 peer 的通知记录（用于 NotifiedPeers JSON 字段）
type peerNotifyRecord struct {
	Count    int   `json:"c"` // 当天通知次数
	LastTime int64 `json:"t"` // 最后通知时间戳
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

const (
	maxNotifyPerDay = 3   // 同一设备每天最多通知次数
	notifyCooldown  = 600 // 单个 peer 最短通知间隔（秒），避免短时间内重复触发
)

type AlertService struct{}

func (s *AlertService) StartChecker() {
	AllService.AlertService = s
		go func() {
		for {
			s.checkOfflineDevices()
			time.Sleep(30 * time.Minute)
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

	// 按用户分组处理：每个用户的告警配置各自独立
	// key=userId, value=用户的station配置（若存在）
	userStationCfg := make(map[uint]*model.AlertConfig)
	for i := range configs {
		if configs[i].Channel == "station" {
			userStationCfg[configs[i].UserId] = &configs[i]
		}
	}

	for _, cfg := range configs {
		if cfg.Channel == "station" {
			continue
		}
		threshold := int64(cfg.OfflineMin * 60)
		if threshold <= 0 {
			threshold = 300
		}

		peerIds, monitorAll := s.getMonitoredPeerIds(&cfg)

		// 如果该配置有过离线通知记录，检查是否有设备重新上线
		// 有则重置 last_notified_at，下次离线可立即通知
		if cfg.LastNotifiedAt > 0 {
			var onlineCount int64
			onlineQuery := DB.Model(&model.Peer{}).Where("last_online_time > ?", now-300)
			if !monitorAll && len(peerIds) > 0 {
				onlineQuery = onlineQuery.Where("id in (?)", peerIds)
			}
			onlineQuery.Count(&onlineCount)
			if onlineCount > 0 {
				DB.Model(&model.AlertConfig{}).Where("row_id = ?", cfg.RowId).Update("last_notified_at", 0)
				cfg.LastNotifiedAt = 0
			}
		}

		var offlinePeers []model.Peer
		// 查询离线设备：最近1小时内离线且超过阈值
		// 策略2：连续3天以上离线的不再通知（已由 last_online_time > 1h 隐式覆盖）
		oneHourAgo := now - 3600
		query := DB.Where("last_online_time > ? AND last_online_time < ?", oneHourAgo, now-threshold)
		if !monitorAll && len(peerIds) > 0 {
			query = query.Where("id in (?)", peerIds)
		} else if !monitorAll {
			continue
		}
		query.Limit(10).Find(&offlinePeers)

		if len(offlinePeers) == 0 {
			continue
		}

		// 解析 NotifiedPeers
		notifiedMap := parseNotifiedPeers(cfg.NotifiedPeers)

		// 筛选出可以通知的 peer（排除已达到每天上限或冷却期内的）
		var newOfflinePeers []model.Peer
		for _, peer := range offlinePeers {
			rec, exists := notifiedMap[peer.Id]
			if exists {
				// 策略1：同一天已通知3次，当天不再通知
				if isSameDay(rec.LastTime, now) && rec.Count >= maxNotifyPerDay {
					continue
				}
				// 最短冷却间隔：10分钟内不重复通知同一设备
				if now-rec.LastTime < notifyCooldown {
					continue
				}
			}
			newOfflinePeers = append(newOfflinePeers, peer)
		}

		if len(newOfflinePeers) == 0 {
			continue
		}
		// 配置级别兜底：1小时内已发过批次通知则跳过
		if now-cfg.LastNotifiedAt < 3600 {
			continue
		}
		for _, peer := range newOfflinePeers {
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
				content := fmt.Sprintf("设备：%s\n别名：%s\nID：%s\n离线时长：%d 分钟\n最后在线：%s",
					hostname, alias, peer.Id, cfg.OfflineMin, lastOnline)

				// 发送外部渠道通知
				AllService.NotifyService.SendByConfig(&cfg, title, content)

				// 该用户是否有站内消息配置？有则发站内消息
				if stationCfg, ok := userStationCfg[cfg.UserId]; ok && stationCfg != nil {
					AllService.NotifyService.SendStationMessage(cfg.UserId, title, content, peer.Id)
				}

				// 更新该 peer 的通知记录
				rec, exists := notifiedMap[peer.Id]
				if exists && isSameDay(rec.LastTime, now) {
					rec.Count++
					rec.LastTime = now
				} else {
					notifiedMap[peer.Id] = &peerNotifyRecord{Count: 1, LastTime: now}
				}
			}
			// 清理非今天的记录，避免 map 无限增长
			for k, v := range notifiedMap {
				if !isSameDay(v.LastTime, now) {
					delete(notifiedMap, k)
				}
			}
			// 持久化 NotifiedPeers
			encoded, _ := json.Marshal(notifiedMap)
			DB.Model(&model.AlertConfig{}).Where("row_id = ?", cfg.RowId).Updates(map[string]interface{}{
				"last_notified_at": now,
				"notified_peers":   string(encoded),
			})
	}
}
