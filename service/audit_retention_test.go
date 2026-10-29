package service

import (
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/model"
	logrus "github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupRetentionTestDB 初始化内存 SQLite，只迁移审计相关三张表。
//
// 注意：sqlite 驱动需要 CGO。CGO_ENABLED=0 的环境（如未装 gcc 的 Windows
// 开发机）会拿到 "stub" 驱动，这里直接 Skip 而不是 Fail —— 该环境限制与
// 本功能无关（同目录 alert_test.go 也依赖 sqlite）。
func setupRetentionTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite 不可用（需要 CGO）: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1) // 内存库需单连接
	}
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}, &model.PunchDirectOk{}); err != nil {
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

// 超过保留天数的记录被删除，保留期内的记录必须保留，且三张表都被覆盖。
func TestAuditRetentionPrunesOldRecords(t *testing.T) {
	db := setupRetentionTestDB(t)
	Config.Audit.RetentionDays = 90

	old := time.Now().AddDate(0, 0, -120) // 120 天前，应被删
	recent := time.Now().AddDate(0, 0, -10)

	// 直接写 created_at 需要绕过 AutoTime 的自动填充，用 Exec 更可靠
	for _, q := range []string{
		"INSERT INTO audit_conns (peer_id, from_peer, conn_id, created_at, updated_at) VALUES ('b', 'a', 1, ?, ?)",
		"INSERT INTO audit_files (peer_id, from_peer, conn_id, created_at, updated_at) VALUES ('b', 'a', 2, ?, ?)",
		"INSERT INTO punch_direct_oks (peer_id, target, conn_type, created_at, updated_at) VALUES ('b', 'a', 'direct_v4', ?, ?)",
	} {
		if err := db.Exec(q, old, old).Error; err != nil {
			t.Fatalf("insert old: %v", err)
		}
		if err := db.Exec(q, recent, recent).Error; err != nil {
			t.Fatalf("insert recent: %v", err)
		}
	}

	s := &AuditRetentionService{}
	deleted := s.pruneOnce()
	if deleted != 3 {
		t.Fatalf("expected 3 deleted (one per table), got %d", deleted)
	}

	check := func(name string, m interface{}) {
		var n int64
		DB.Model(m).Count(&n)
		if n != 1 {
			t.Fatalf("%s: expected 1 remaining (recent), got %d", name, n)
		}
	}
	check("audit_conns", &model.AuditConn{})
	check("audit_files", &model.AuditFile{})
	check("punch_direct_oks", &model.PunchDirectOk{})
}

// 未配置 retention-days 时应回退到默认 90 天（不是 0，即不会清空）
func TestAuditRetentionDefaultDays(t *testing.T) {
	setupRetentionTestDB(t)
	Config.Audit.RetentionDays = 0 // 走默认

	// 100 天前：超过默认 90 天，应被删
	if err := DB.Exec(
		"INSERT INTO audit_conns (peer_id, from_peer, conn_id, created_at, updated_at) VALUES ('b','a',9,?,?)",
		time.Now().AddDate(0, 0, -100), time.Now(),
	).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	// 30 天前：保留期内，必须保留（若误用 0 天会把它删掉）
	if err := DB.Exec(
		"INSERT INTO audit_conns (peer_id, from_peer, conn_id, created_at, updated_at) VALUES ('b','a',10,?,?)",
		time.Now().AddDate(0, 0, -30), time.Now(),
	).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	if deleted := (&AuditRetentionService{}).pruneOnce(); deleted != 1 {
		t.Fatalf("expected 1 deleted by default 90d policy, got %d", deleted)
	}
	var n int64
	DB.Model(&model.AuditConn{}).Count(&n)
	if n != 1 {
		t.Fatalf("expected 30-day-old record kept, remaining=%d", n)
	}
}
