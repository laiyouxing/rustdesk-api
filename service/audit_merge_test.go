package service

import (
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

// 同一次操作的第二次尝试（120s 内、同 from_peer → 同 peer_id）应并入第一条记录：
// 只剩一条、conn_id 换成最新值（保证后续 close 审计能命中）、attempt_count=2、仍为进行中。
func TestMergeConnAttempt(t *testing.T) {
	db := setupPeerAuditTestDB(t)
	s := &AuditService{}

	primary := &model.AuditConn{Action: model.AuditActionNew, PeerId: "peer-1", FromPeer: "peer-2",
		FromName: "张三", ConnId: 1, Ip: "1.2.3.4"}
	if err := db.Create(primary).Error; err != nil {
		t.Fatalf("create primary: %v", err)
	}
	// 第二次尝试：被控端先收到 new，落了一行（conn_id=2，还没有 from_peer）
	curRow := &model.AuditConn{Action: model.AuditActionNew, PeerId: "peer-1", ConnId: 2, Ip: "1.2.3.4"}
	db.Create(curRow)

	cur := &model.AuditConn{PeerId: "peer-1", FromPeer: "peer-2", FromName: "张三", ConnId: 2,
		SessionId: "12345", Ip: "1.2.3.4"}
	merged := s.MergeConnAttempt(cur)
	if merged == 0 {
		t.Fatalf("应合并进已有记录，实际返回 0")
	}

	var rows []model.AuditConn
	db.Where("peer_id = ? and from_peer = ?", "peer-1", "peer-2").Find(&rows)
	if len(rows) != 1 {
		t.Fatalf("合并后应只剩 1 条记录，实际 %d 条", len(rows))
	}
	got := rows[0]
	if got.ConnId != 2 {
		t.Errorf("conn_id 应更新为最新的 2，实际 %d", got.ConnId)
	}
	if got.AttemptCount != 2 {
		t.Errorf("attempt_count 应为 2，实际 %d", got.AttemptCount)
	}
	if got.CloseTime != 0 {
		t.Errorf("同一次操作的重试应保持进行中，实际 close_time=%d", got.CloseTime)
	}
	if got.SessionId != "12345" {
		t.Errorf("session_id 应更新为最新值，实际 %q", got.SessionId)
	}
	if got.FromName != "张三" {
		t.Errorf("from_name 不应丢失，实际 %q", got.FromName)
	}
	// 本次尝试临时建的行应被删除
	var n int64
	db.Model(&model.AuditConn{}).Where("peer_id = ? and conn_id = ?", "peer-1", 2).Count(&n)
	if n != 1 { // 就是合并后的主记录本身
		t.Errorf("临时行应被清理，同 conn_id 记录数=%d", n)
	}
}

// 上一次连接早已结束（超出合并窗口）时，不应连成一条——用户是两次独立的操作。
func TestMergeConnAttemptOutOfWindow(t *testing.T) {
	db := setupPeerAuditTestDB(t)
	s := &AuditService{}

	old := &model.AuditConn{Action: model.AuditActionNew, PeerId: "peer-1", FromPeer: "peer-2",
		ConnId: 1, CloseTime: time.Now().Add(-time.Hour).Unix()}
	db.Create(old)

	if got := s.MergeConnAttempt(&model.AuditConn{PeerId: "peer-1", FromPeer: "peer-2", ConnId: 9}); got != 0 {
		t.Fatalf("超出窗口不应合并，实际合并到 %d", got)
	}
}

// 同一次操作里没登录成功的尝试（from_peer 为空、同控制端 IP、窗口内）应被吸收掉，
// 否则后台会留下几条「没有来源用户」的重复记录。
func TestMergeConnAttemptDropsFailedAttempts(t *testing.T) {
	db := setupPeerAuditTestDB(t)
	s := &AuditService{}

	db.Create(&model.AuditConn{Action: model.AuditActionNew, PeerId: "peer-1", FromPeer: "peer-2",
		FromName: "张三", ConnId: 1, Ip: "1.2.3.4"})
	// 打洞失败留下的两条：没有 from_peer
	db.Create(&model.AuditConn{Action: model.AuditActionNew, PeerId: "peer-1", ConnId: 2, Ip: "1.2.3.4", CloseTime: time.Now().Unix()})
	db.Create(&model.AuditConn{Action: model.AuditActionNew, PeerId: "peer-1", ConnId: 3, Ip: "1.2.3.4"})

	s.MergeConnAttempt(&model.AuditConn{PeerId: "peer-1", FromPeer: "peer-2", FromName: "张三",
		ConnId: 4, Ip: "1.2.3.4"})

	var rows []model.AuditConn
	db.Where("peer_id = ?", "peer-1").Find(&rows)
	if len(rows) != 1 {
		t.Fatalf("失败尝试应被吸收，剩余记录数=%d", len(rows))
	}
	if rows[0].FromPeer != "peer-2" || rows[0].ConnId != 4 {
		t.Errorf("保留的应是主记录且 conn_id=4，实际 from_peer=%q conn_id=%d", rows[0].FromPeer, rows[0].ConnId)
	}
}

// 不同来源设备（另一个用户）连同一台被控端时，不能被合并掉。
func TestMergeConnAttemptKeepsOtherUsers(t *testing.T) {
	db := setupPeerAuditTestDB(t)
	s := &AuditService{}

	db.Create(&model.AuditConn{Action: model.AuditActionNew, PeerId: "peer-1", FromPeer: "peer-2", ConnId: 1, Ip: "1.2.3.4"})
	db.Create(&model.AuditConn{Action: model.AuditActionNew, PeerId: "peer-1", FromPeer: "peer-3", ConnId: 2, Ip: "5.6.7.8"})

	if got := s.MergeConnAttempt(&model.AuditConn{PeerId: "peer-1", FromPeer: "peer-3", ConnId: 2, Ip: "5.6.7.8"}); got != 0 {
		t.Fatalf("不同用户不应互相合并，实际合并到 %d", got)
	}
	var n int64
	db.Model(&model.AuditConn{}).Where("peer_id = ?", "peer-1").Count(&n)
	if n != 2 {
		t.Fatalf("两家控制端的记录都应保留，实际 %d 条", n)
	}
}
