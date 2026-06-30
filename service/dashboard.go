package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
	"time"
)

type DashboardStats struct {
	TotalPeers       int64 `json:"total_peers"`
	OnlinePeers      int64 `json:"online_peers"`
	OfflinePeers     int64 `json:"offline_peers"`
	TotalUsers       int64 `json:"total_users"`
	TodayConnections int64 `json:"today_connections"`
}

type DashboardService struct{}

func (s *DashboardService) Stats() *DashboardStats {
	stats := &DashboardStats{}
	now := time.Now().Unix()

	DB.Model(&model.Peer{}).Count(&stats.TotalPeers)
	DB.Model(&model.Peer{}).Where("last_online_time > ?", now-300).Count(&stats.OnlinePeers)
	DB.Model(&model.Peer{}).Where("last_online_time <= ? OR last_online_time = 0", now-300).Count(&stats.OfflinePeers)
	DB.Model(&model.User{}).Count(&stats.TotalUsers)

	todayStart := time.Now().Truncate(24 * time.Hour).Unix()
	DB.Model(&model.LoginLog{}).Where("created_at > ?", todayStart).Count(&stats.TodayConnections)

	return stats
}

// UserStats 返回指定用户的设备统计数据
func (s *DashboardService) UserStats(userId uint) *DashboardStats {
	stats := &DashboardStats{}
	now := time.Now().Unix()

	DB.Model(&model.Peer{}).Where("user_id = ?", userId).Count(&stats.TotalPeers)
	DB.Model(&model.Peer{}).Where("user_id = ? and last_online_time > ?", userId, now-300).Count(&stats.OnlinePeers)
	DB.Model(&model.Peer{}).Where("user_id = ? and (last_online_time <= ? OR last_online_time = 0)", userId, now-300).Count(&stats.OfflinePeers)
	// 普通用户不显示总用户数
	stats.TotalUsers = 0
	todayStart := time.Now().Truncate(24 * time.Hour).Unix()
	DB.Model(&model.LoginLog{}).Where("user_id = ? and created_at > ?", userId, todayStart).Count(&stats.TodayConnections)

	return stats
}
