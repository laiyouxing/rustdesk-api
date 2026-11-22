package service

import (
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
	logrus "github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupNodeTestDB 初始化内存 SQLite，只迁移节点与系统参数两张表。
//
// 与 audit_retention_test.go 一致：sqlite 驱动需要 CGO，CGO_ENABLED=0 的
// 环境会拿到 stub 驱动，这里 Skip 而不是 Fail。
func setupNodeTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite 不可用（需要 CGO）: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1) // 内存库需单连接
	}
	if err := db.AutoMigrate(&model.ServerNode{}, &model.SystemSetting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	oldDB, oldLog, oldAll := DB, Logger, AllService
	DB = db
	Logger = logrus.New()
	AllService = &Service{}
	AllService.SettingService = &SettingService{}
	AllService.NodeService = &NodeService{}
	t.Cleanup(func() {
		DB, Logger, AllService = oldDB, oldLog, oldAll
	})
	return db
}

// seedRelay 插入一台在线 hbbr 节点。
func seedRelay(t *testing.T, db *gorm.DB, addr string, disabled int, weight int) {
	t.Helper()
	n := model.ServerNode{
		NodeId:   "hbbr@" + addr,
		Kind:     "hbbr",
		Addr:     addr,
		Weight:   weight,
		Disabled: disabled,
		Status:   1,
		RttMs:    -1,
		LastSeen: time.Now().Unix(),
	}
	if err := db.Create(&n).Error; err != nil {
		t.Fatalf("create node %s: %v", addr, err)
	}
}

// relayAddrs 取出 relays 列表里的地址（保持顺序）。
func relayAddrs(list []map[string]interface{}) []string {
	out := make([]string, 0, len(list))
	for _, it := range list {
		if a, ok := it["addr"].(string); ok {
			out = append(out, a)
		}
	}
	return out
}

// TestRelaysSkipsDisabledNodeInStaticFallback 回归：管理员下线的中继不能被
// 静态兜底列表"复活"。
//
// 原缺陷：Relays() 在拼完调度结果后无条件追加 StaticRelays()，只按地址去重，
// 不看节点是否 disabled。于是后台点了"手动下线"，调度列表里依然出现该地址，
// 下线看起来完全没生效。
func TestRelaysSkipsDisabledNodeInStaticFallback(t *testing.T) {
	db := setupNodeTestDB(t)
	seedRelay(t, db, "1.1.1.1:21117", 0, 100)
	seedRelay(t, db, "2.2.2.2:21117", 1, 100) // 已下线

	// 静态兜底里同时包含在线与已下线两台（模拟历史配置没清理）
	if err := AllService.SettingService.Set(SettingKeyNodeStaticRelays,
		"1.1.1.1:21117,2.2.2.2:21117,3.3.3.3:21117"); err != nil {
		t.Fatalf("set static relays: %v", err)
	}

	got := relayAddrs(AllService.NodeService.Relays("", 0))
	for _, a := range got {
		if a == "2.2.2.2:21117" {
			t.Fatalf("已下线的中继仍出现在调度列表里: %v", got)
		}
	}
	// 在线节点与未注册的静态地址都应保留
	want := map[string]bool{"1.1.1.1:21117": false, "3.3.3.3:21117": false}
	for _, a := range got {
		if _, ok := want[a]; ok {
			want[a] = true
		}
	}
	for a, seen := range want {
		if !seen {
			t.Fatalf("期望 %s 出现在调度列表，实际 %v", a, got)
		}
	}
}

// TestRelaysSortsByScore 调度列表必须按分数升序，人工权重大的排在前面。
func TestRelaysSortsByScore(t *testing.T) {
	db := setupNodeTestDB(t)
	// 注意：weight 列带 gorm:"default:100"，Create 时零值会被默认值覆盖，
	// 所以这里用 10/100 而不是 0/100（0 只能通过 Update 写入）。
	seedRelay(t, db, "1.1.1.1:21117", 0, 10)  // 人工权重低 → 成本高
	seedRelay(t, db, "2.2.2.2:21117", 0, 100) // 人工权重高 → 成本低

	got := relayAddrs(AllService.NodeService.Relays("", 0))
	if len(got) != 2 {
		t.Fatalf("期望 2 台中继，实际 %v", got)
	}
	if got[0] != "2.2.2.2:21117" {
		t.Fatalf("人工权重高的节点应排第一，实际顺序 %v", got)
	}
}

// TestDefaultWeightsGiveManualWeightEffect 默认权重里人工权重系数必须非 0，
// 否则改「人工权重」对分数没有任何影响（"调了没效果"的直接原因）。
func TestDefaultWeightsGiveManualWeightEffect(t *testing.T) {
	w := DefaultNodeScoreWeights()
	if w.Weight <= 0 {
		t.Fatalf("默认人工权重系数为 0，人工权重不会生效: %+v", w)
	}
	sum := w.Sessions + w.Bandwidth + w.CPU + w.Mem + w.RTT + w.Weight
	if sum < 0.999 || sum > 1.001 {
		t.Fatalf("默认权重总和应约为 1，实际 %v: %+v", sum, w)
	}
}

// TestScoreWeightSaturatesAtMax 人工权重超过上限后不再有区分度（100 与 10000 等价）。
func TestScoreWeightSaturatesAtMax(t *testing.T) {
	// Score 不读接收者字段，直接用零值实例即可，避免依赖全局 AllService
	svc := &NodeService{}
	w := NodeScoreWeights{Weight: 1}
	base := model.ServerNode{MaxSessions: 0, RttMs: -1}

	atMax := base
	atMax.Weight = MaxNodeManualWeight
	above := base
	above.Weight = MaxNodeManualWeight * 100

	// RttMs=-1 时 sRtt=1，Weight 系数 1，成本项唯一，可直接比较
	if got, want := svc.Score(&above, w), svc.Score(&atMax, w); got != want {
		t.Fatalf("超过上限后应饱和：weight=%d 得 %v，weight=%d 得 %v",
			above.Weight, got, atMax.Weight, want)
	}
	low := base
	low.Weight = 0
	if svc.Score(&low, w) <= svc.Score(&atMax, w) {
		t.Fatalf("人工权重 0 的成本应高于上限值")
	}
}

// TestDisabledRelayAddrs 下线地址列表供 hbbs 剔除保留的旧列表。
func TestDisabledRelayAddrs(t *testing.T) {
	db := setupNodeTestDB(t)
	seedRelay(t, db, "1.1.1.1:21117", 0, 100)
	seedRelay(t, db, "2.2.2.2:21117", 1, 100)

	got := AllService.NodeService.DisabledRelayAddrs()
	if len(got) != 1 || got[0] != "2.2.2.2:21117" {
		t.Fatalf("期望只返回已下线地址，实际 %v", got)
	}
}
