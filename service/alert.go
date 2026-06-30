package service

import (
	"fmt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"time"
)

type AlertService struct{}

func (s *AlertService) StartChecker() {
	AllService.AlertService = s
	go func() {
		for {
			s.checkOfflineDevices()
			time.Sleep(3 * time.Minute)
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

		var offlinePeers []model.Peer
		query := DB.Where("last_online_time > 0 AND last_online_time < ?", now-threshold)
		if !monitorAll && len(peerIds) > 0 {
			query = query.Where("id in (?)", peerIds)
		} else if !monitorAll {
			continue
		}
		query.Limit(10).Find(&offlinePeers)

		for _, peer := range offlinePeers {
			hostname := peer.Hostname
			if hostname == "" {
				hostname = peer.Id
			}
			title := "设备离线告警"
			content := fmt.Sprintf("设备 %s (ID: %s) 已离线超过 %d 分钟", hostname, peer.Id, cfg.OfflineMin)

			// 去重检查：上次通知时间距今是否超过一个检测周期(3min)
			if cfg.LastNotifiedAt > now-180 {
				continue
			}

			// 发送外部渠道通知
			AllService.NotifyService.SendByConfig(&cfg, title, content)

			// 该用户是否有站内消息配置？有则发站内消息（只发给该用户自己）
			if stationCfg, ok := userStationCfg[cfg.UserId]; ok && stationCfg != nil {
				AllService.NotifyService.SendStationMessage(cfg.UserId, title, content, peer.Id)
			}

			// 更新上次通知时间，避免重复触发
			DB.Model(&model.AlertConfig{}).Where("row_id = ?", cfg.RowId).Update("last_notified_at", now)
		}
	}
}
