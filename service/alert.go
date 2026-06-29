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

// getMonitoredPeerIds returns peer IDs that should be monitored by this config.
// If monitor_all=1, returns the user's own address book peer IDs (personal scope).
// If monitor_all=2, returns the selected peer IDs from alert_targets.
func (s *AlertService) getMonitoredPeerIds(cfg *model.AlertConfig) ([]string, bool) {
	if cfg.MonitorAll == 1 {
		// Personal scope: only the user's own devices
		// Get peer IDs from user's address book (including own + shared)
		var abEntries []model.AddressBook

		// Own collections
		var ownColls []model.AddressBookCollection
		DB.Where("user_id = ?", cfg.UserId).Find(&ownColls)
		ownCollIds := []uint{0} // collection_id=0 is personal default
		for _, col := range ownColls {
			ownCollIds = append(ownCollIds, col.Id)
		}

		// Shared collections (personally or via group)
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

		// Get all address book entries for these collections
		DB.Where("collection_id in (?)", ownCollIds).Find(&abEntries)
		if len(abEntries) == 0 {
			return nil, true // fallback to all peers if no address book
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
		// No targets configured -> use personal scope too
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

	var stationCfg *model.AlertConfig
	for i := range configs {
		if configs[i].Channel == "station" {
			stationCfg = &configs[i]
			break
		}
	}

	now := time.Now().Unix()

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
			continue // no targets to check
		}
		query.Limit(10).Find(&offlinePeers)

		for _, peer := range offlinePeers {
			hostname := peer.Hostname
			if hostname == "" {
				hostname = peer.Id
			}
			title := "设备离线告警"
			content := fmt.Sprintf("设备 %s (ID: %s) 已离线超过 %d 分钟", hostname, peer.Id, cfg.OfflineMin)
			AllService.NotifyService.SendByConfig(&cfg, title, content)
			if stationCfg != nil {
				AllService.NotifyService.SendStationMessage(title, content, peer.Id)
			}
		}
	}
}
