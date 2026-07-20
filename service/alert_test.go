package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAlertTestDB 初始化内存 SQLite 与全局依赖，并返回 db。
// 每个测试独立建库，互不干扰；测试结束还原全局变量以免污染同包其它测试。
func setupAlertTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1) // 内存库需单连接，避免各连接独立成库
	}
	if err := db.AutoMigrate(&model.Peer{}, &model.AlertConfig{}, &model.AlertTarget{}, &model.StationMessage{}, &model.AlertChannel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	oldDB, oldLog, oldAll := DB, Logger, AllService
	DB = db
	Logger = logrus.New()
	AllService = &Service{}
	AllService.AlertService = &AlertService{}
	AllService.NotifyService = &NotifyService{}
	t.Cleanup(func() {
		DB, Logger, AllService = oldDB, oldLog, oldAll
	})
	return db
}

type seedOpts struct {
	offlineMin     int
	peerLastOnline int64 // 绝对时间戳；0 表示按 offlineMin 推算为“已离线超过阈值”
	consecutive    int   // 连续触发天数（预置）
	lastTriggerDay int   // 最近触发日期（预置）
	notifiedPeers  string
	enabled        int
}

// seedScenario 构造：一个离线设备 + 一条 smtp 告警规则(指向该设备) + 一条 station 规则(同用户，用于计数推送)。
// 返回 smtp 配置的主键与设备 id。
func seedScenario(t *testing.T, db *gorm.DB, o seedOpts) (uint, string) {
	peerId := "peer-1"
	lastOnline := o.peerLastOnline
	if lastOnline == 0 {
		lastOnline = time.Now().Unix() - int64(o.offlineMin)*60 - 600 // 远超阈值，判定为离线
	}
	if o.enabled == 0 {
		o.enabled = 1
	}
	peer := model.Peer{Id: peerId, Hostname: "host-1", Alias: "alias-1", UserId: 1, LastOnlineTime: lastOnline}
	if err := db.Create(&peer).Error; err != nil {
		t.Fatalf("create peer: %v", err)
	}

	cfg := model.AlertConfig{
		UserId:                1,
		Channel:               "smtp",
		ChannelId:             0, // 无通道行 -> SendByConfig 安全空操作，不会发起网络
		Name:                  "smtp-cfg",
		OfflineMin:            o.offlineMin,
		Enabled:               o.enabled,
		MonitorAll:            2,
		ConsecutiveTriggerDays: o.consecutive,
		LastTriggerDay:        o.lastTriggerDay,
		NotifiedPeers:         o.notifiedPeers,
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatalf("create cfg: %v", err)
	}
	if err := db.Create(&model.AlertTarget{AlertId: cfg.RowId, TargetType: "peer", TargetId: peerId, TargetName: peerId}).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	// 同用户 station 规则：仅用于 populate userStationCfg，使推送时写入 station_messages 以便计数
	if err := db.Create(&model.AlertConfig{UserId: 1, Channel: "station", ChannelId: 0, Name: "station-cfg", OfflineMin: o.offlineMin, Enabled: 1, MonitorAll: 2}).Error; err != nil {
		t.Fatalf("create station cfg: %v", err)
	}
	return cfg.RowId, peerId
}

func npJSON(weight, weightDay, count int, lastTime int64) string {
	m := map[string]*peerNotifyRecord{
		"peer-1": {Count: count, LastTime: lastTime, Weight: weight, WeightDay: weightDay},
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func countStationMessages(t *testing.T, db *gorm.DB) int64 {
	var n int64
	if err := db.Model(&model.StationMessage{}).Count(&n).Error; err != nil {
		t.Fatalf("count station messages: %v", err)
	}
	return n
}

func getCfg(t *testing.T, db *gorm.DB, rowId uint) model.AlertConfig {
	var c model.AlertConfig
	if err := db.Where("row_id = ?", rowId).First(&c).Error; err != nil {
		t.Fatalf("get cfg: %v", err)
	}
	return c
}

// TestAlert_WeightAccumulation 验证：每 5 分钟检测一次，离线则权重+1，
// 权重累积到 10 才触发一次推送（之后冷却期阻止重复推送）。
func TestAlert_WeightAccumulation(t *testing.T) {
	db := setupAlertTestDB(t)
	cfgRowId, _ := seedScenario(t, db, seedOpts{offlineMin: 5})

	for i := 0; i < 11; i++ {
		AllService.AlertService.checkOfflineDevices()
	}

	if got := countStationMessages(t, db); got != 1 {
		t.Fatalf("权重达到 10 应推送 1 次，实际 %d 次", got)
	}
	rec := parseNotifiedPeers(getCfg(t, db, cfgRowId).NotifiedPeers)["peer-1"]
	if rec == nil {
		t.Fatalf("未找到 peer-1 的通知记录")
	}
	if rec.Weight < offlineWeightThreshold {
		t.Fatalf("权重应 >= %d，实际 %d", offlineWeightThreshold, rec.Weight)
	}
}

// TestAlert_ThresholdNotReachedNoPush 验证：权重未达阈值不推送。
func TestAlert_ThresholdNotReachedNoPush(t *testing.T) {
	db := setupAlertTestDB(t)
	seedScenario(t, db, seedOpts{offlineMin: 5})
	// 仅检测 9 次，权重最高到 9，不足 10
	for i := 0; i < 9; i++ {
		AllService.AlertService.checkOfflineDevices()
	}
	if got := countStationMessages(t, db); got != 0 {
		t.Fatalf("权重未达阈值不应推送，实际 %d 次", got)
	}
}

// TestAlert_DailyWeightReset 验证：跨天重置权重（WeightDay 与今日不同则归零后重新累积）。
func TestAlert_DailyWeightReset(t *testing.T) {
	db := setupAlertTestDB(t)
	today := dayKey(time.Now().Unix())
	yesterday := dayKey(time.Now().Unix() - 86400)
	cfgRowId, _ := seedScenario(t, db, seedOpts{
		offlineMin:    5,
		notifiedPeers: npJSON(5, yesterday, 0, 0), // 昨天累积的权重 5
	})
	AllService.AlertService.checkOfflineDevices()

	rec := parseNotifiedPeers(getCfg(t, db, cfgRowId).NotifiedPeers)["peer-1"]
	if rec == nil {
		t.Fatalf("未找到通知记录")
	}
	if rec.Weight != 1 {
		t.Fatalf("每日重置后权重应为 1（归零后本次 +1），实际 %d", rec.Weight)
	}
	if rec.WeightDay != today {
		t.Fatalf("WeightDay 应为今日 %d，实际 %d", today, rec.WeightDay)
	}
}

// TestAlert_OnlineResetsWeight 验证：设备上线即重置其离线权重。
func TestAlert_OnlineResetsWeight(t *testing.T) {
	db := setupAlertTestDB(t)
	today := dayKey(time.Now().Unix())
	cfgRowId, _ := seedScenario(t, db, seedOpts{
		offlineMin:     5,
		peerLastOnline: time.Now().Unix(), // 当前在线
		notifiedPeers:  npJSON(5, today, 0, 0),
	})
	AllService.AlertService.checkOfflineDevices()

	rec := parseNotifiedPeers(getCfg(t, db, cfgRowId).NotifiedPeers)["peer-1"]
	if rec == nil {
		t.Fatalf("未找到通知记录")
	}
	if rec.Weight != 0 {
		t.Fatalf("上线后权重应重置为 0，实际 %d", rec.Weight)
	}
	if got := countStationMessages(t, db); got != 0 {
		t.Fatalf("在线设备不应推送，实际 %d 次", got)
	}
}

// TestAlert_WithinThresholdNoWeight 验证：刚离线但尚未超过阈值时长，不计入权重。
func TestAlert_WithinThresholdNoWeight(t *testing.T) {
	db := setupAlertTestDB(t)
	today := dayKey(time.Now().Unix())
	// 离线仅 2 分钟，阈值 5 分钟 -> 不应累积权重
	cfgRowId, _ := seedScenario(t, db, seedOpts{
		offlineMin:     5,
		peerLastOnline: time.Now().Unix() - 120,
		notifiedPeers:  npJSON(0, today, 0, 0),
	})
	AllService.AlertService.checkOfflineDevices()

	rec := parseNotifiedPeers(getCfg(t, db, cfgRowId).NotifiedPeers)["peer-1"]
	if rec == nil || rec.Weight != 0 {
		t.Fatalf("阈值内离线不应累积权重，实际 %+v", rec)
	}
	if got := countStationMessages(t, db); got != 0 {
		t.Fatalf("阈值内不应推送，实际 %d 次", got)
	}
}

// TestAlert_ConsecutiveThreeDaysSuppress 验证：连续触发满 3 天后不再推送邮件。
func TestAlert_ConsecutiveThreeDaysSuppress(t *testing.T) {
	db := setupAlertTestDB(t)
	today := dayKey(time.Now().Unix())
	cfgRowId, _ := seedScenario(t, db, seedOpts{
		offlineMin:  5,
		consecutive: 3,                       // 已连续 3 天
		notifiedPeers: npJSON(10, today, 0, 0), // 权重已达标，本应触发
	})
	AllService.AlertService.checkOfflineDevices()

	if got := countStationMessages(t, db); got != 0 {
		t.Fatalf("连续3天以上应停止推送，实际 %d 次", got)
	}
	if c := getCfg(t, db, cfgRowId); c.ConsecutiveTriggerDays != 3 {
		t.Fatalf("抑制期间连续天数应保持 3，实际 %d", c.ConsecutiveTriggerDays)
	}
}

// TestAlert_ReconnectResetsConsecutive 验证：设备重新上线重置“连续3天”限制。
func TestAlert_ReconnectResetsConsecutive(t *testing.T) {
	db := setupAlertTestDB(t)
	cfgRowId, _ := seedScenario(t, db, seedOpts{
		offlineMin:     5,
		consecutive:    3,
		peerLastOnline: time.Now().Unix(), // 在线 -> 触发重置
	})
	AllService.AlertService.checkOfflineDevices()

	if c := getCfg(t, db, cfgRowId); c.ConsecutiveTriggerDays != 0 || c.LastTriggerDay != 0 {
		t.Fatalf("重新上线应重置连续限制，实际 days=%d lastDay=%d", c.ConsecutiveTriggerDays, c.LastTriggerDay)
	}
}

// TestAlert_MaxThreePerDay 验证：同一设备当天已达 3 次上限，不再推送。
func TestAlert_MaxThreePerDay(t *testing.T) {
	db := setupAlertTestDB(t)
	today := dayKey(time.Now().Unix())
	cfgRowId, _ := seedScenario(t, db, seedOpts{
		offlineMin:     5,
		notifiedPeers:  npJSON(10, today, maxNotifyPerDay, time.Now().Unix()), // 当天已 3 次
	})
	AllService.AlertService.checkOfflineDevices()

	if got := countStationMessages(t, db); got != 0 {
		t.Fatalf("当天已达上限不应推送，实际 %d 次", got)
	}
	rec := parseNotifiedPeers(getCfg(t, db, cfgRowId).NotifiedPeers)["peer-1"]
	if rec == nil || rec.Count != maxNotifyPerDay {
		t.Fatalf("已达上限时 Count 应保持 %d，实际 %+v", maxNotifyPerDay, rec)
	}
}

// TestAlert_Cooldown 验证：冷却期内（与上一次推送间隔 < notifyCooldown）不重复推送。
func TestAlert_Cooldown(t *testing.T) {
	db := setupAlertTestDB(t)
	today := dayKey(time.Now().Unix())
	cfgRowId, _ := seedScenario(t, db, seedOpts{
		offlineMin:     5,
		notifiedPeers:  npJSON(10, today, 0, time.Now().Unix()), // 刚刚推送过
	})
	AllService.AlertService.checkOfflineDevices()

	if got := countStationMessages(t, db); got != 0 {
		t.Fatalf("冷却期内不应推送，实际 %d 次", got)
	}
	_ = cfgRowId
}
