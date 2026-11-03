package service

import (
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/model"
	logrus "github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPeerAuditTestDB 内存 SQLite（迁移审计表与 peers 表）。
// 与 setupRetentionTestDB 一样：CGO 不可用时 Skip，因为这是环境限制而非功能问题。
func setupPeerAuditTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite 不可用（需要 CGO）: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}, &model.Peer{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	oldDB, oldLog, oldCfg := DB, Logger, Config
	DB = db
	Logger = logrus.New()
	Config = &config.Config{}
	t.Cleanup(func() {
		DB, Logger, Config = oldDB, oldLog, oldCfg
	})
	return db
}

// 连接日志：缺失的版本/主机名/别名由 peers 表补齐；已有值（连接时的快照）不被覆盖。
func TestFillPeerInfoFromPeers(t *testing.T) {
	db := setupPeerAuditTestDB(t)
	db.Create(&model.Peer{Id: "peer-1", Hostname: "DESKTOP-A", Alias: "桂林五通", Version: "1.7.31"})
	db.Create(&model.Peer{Id: "peer-2", Hostname: "DESKTOP-B", Alias: "南宁机房", Version: "1.7.29"})

	s := &AuditService{}
	c := &model.AuditConn{PeerId: "peer-1", FromPeer: "peer-2"}
	s.FillPeerInfoFromPeers(c)
	if c.Version != "1.7.31" || c.PeerHostname != "DESKTOP-A" || c.PeerAlias != "桂林五通" {
		t.Fatalf("被控端未补齐: %+v", c)
	}
	if c.FromVersion != "1.7.29" || c.FromHostname != "DESKTOP-B" || c.FromAlias != "南宁机房" {
		t.Fatalf("控制端未补齐: %+v", c)
	}

	// 已存的值优先（设备后来改名/升级，不应改写历史记录）
	c2 := &model.AuditConn{
		PeerId: "peer-1", Version: "1.0.0", PeerHostname: "OLD-PC", PeerAlias: "旧别名",
	}
	s.FillPeerInfoFromPeers(c2)
	if c2.Version != "1.0.0" || c2.PeerHostname != "OLD-PC" || c2.PeerAlias != "旧别名" {
		t.Fatalf("已有值被覆盖: %+v", c2)
	}
	if c2.FromVersion != "" || c2.FromHostname != "" {
		t.Fatalf("FromPeer 为空时不应补齐: %+v", c2)
	}

	// peers 表里查不到的设备保持空值，不能报错
	c3 := &model.AuditConn{PeerId: "not-exist"}
	s.FillPeerInfoFromPeers(c3)
	if c3.Version != "" || c3.PeerHostname != "" {
		t.Fatalf("未知设备不应被填值: %+v", c3)
	}
}

// 文件传输日志：单条补齐 + 列表批量补齐。
func TestFillFilePeerInfo(t *testing.T) {
	db := setupPeerAuditTestDB(t)
	db.Create(&model.Peer{Id: "peer-1", Hostname: "DESKTOP-A", Alias: "桂林五通", Version: "1.7.31"})
	db.Create(&model.Peer{Id: "peer-2", Hostname: "DESKTOP-B", Alias: "南宁机房", Version: "1.7.29"})

	s := &AuditService{}
	f := &model.AuditFile{PeerId: "peer-1", FromPeer: "peer-2"}
	s.FillFilePeerInfoFromPeers(f)
	if f.Version != "1.7.31" || f.PeerHostname != "DESKTOP-A" || f.PeerAlias != "桂林五通" {
		t.Fatalf("文件日志被控端未补齐: %+v", f)
	}
	if f.FromVersion != "1.7.29" || f.FromHostname != "DESKTOP-B" || f.FromAlias != "南宁机房" {
		t.Fatalf("文件日志控制端未补齐: %+v", f)
	}

	list := []*model.AuditFile{
		{PeerId: "peer-1", FromPeer: "peer-2"},
		{PeerId: "peer-2", FromPeer: "peer-1"},
		nil,
	}
	s.FillFileListPeerInfo(list)
	if list[0].PeerAlias != "桂林五通" || list[0].FromAlias != "南宁机房" {
		t.Fatalf("列表第 1 条未补齐: %+v", list[0])
	}
	if list[1].PeerAlias != "南宁机房" || list[1].FromVersion != "1.7.31" {
		t.Fatalf("列表第 2 条未补齐: %+v", list[1])
	}
}
