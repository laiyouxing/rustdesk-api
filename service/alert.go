package service

import (
	"fmt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"time"
)

type AlertService struct{}

func (s *AlertService) StartChecker() {
	// Register service in AllService
	AllService.AlertService = s
	go func() {
		for {
			s.checkOfflineDevices()
			time.Sleep(3 * time.Minute)
		}
	}()
	Logger.Info("Alert checker started")
}

func (s *AlertService) checkOfflineDevices() {
	var configs []model.AlertConfig
	DB.Where("enabled = 1").Find(&configs)
	if len(configs) == 0 {
		return
	}

	// Find station message channel
	var stationCfg *model.AlertConfig
	for i := range configs {
		if configs[i].Channel == "station" {
			stationCfg = &configs[i]
			break
		}
	}

	now := time.Now().Unix()

	// Check each enabled config (except station, which stores messages)
	for _, cfg := range configs {
		if cfg.Channel == "station" {
			continue
		}
		threshold := int64(cfg.OfflineMin * 60)
		if threshold <= 0 {
			threshold = 300
		}

		var offlinePeers []model.Peer
		DB.Where("last_online_time > 0 AND last_online_time < ?", now-threshold).
			Limit(10).Find(&offlinePeers)

		for _, peer := range offlinePeers {
			hostname := peer.Hostname
			if hostname == "" {
				hostname = peer.Id
			}
			title := "设备离线告警"
			content := fmt.Sprintf("设备 %s (ID: %s) 已离线超过 %d 分钟", hostname, peer.Id, cfg.OfflineMin)
			// Send to external channel
			AllService.NotifyService.SendByConfig(&cfg, title, content)
			// Also save station message
			if stationCfg != nil {
				AllService.NotifyService.SendStationMessage(title, content, peer.Id)
			}
		}
	}
}
