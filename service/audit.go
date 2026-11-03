package service

import (
	"fmt"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AuditService struct {
}

func (as *AuditService) AuditConnList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AuditConnList) {
	res = &model.AuditConnList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AuditConn{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.AuditConns)
	return
}

// Create 创建
func (as *AuditService) CreateAuditConn(u *model.AuditConn) error {
	res := DB.Create(u).Error
	return res
}
func (as *AuditService) DeleteAuditConn(u *model.AuditConn) error {
	return DB.Delete(u).Error
}

// Update 更新
func (as *AuditService) UpdateAuditConn(u *model.AuditConn) error {
	return DB.Model(u).Updates(u).Error
}

// InfoByPeerIdAndConnId
func (as *AuditService) InfoByPeerIdAndConnId(peerId string, connId int64) (res *model.AuditConn) {
	res = &model.AuditConn{}
	DB.Where("peer_id = ? and conn_id = ?", peerId, connId).First(res)
	return
}

// UpsertByPeerIdAndConnId 按 (peer_id, conn_id) 更新已存在记录的非零字段；不存在则创建。
// 客户端连接建立时先发 new（不带 peer 信息），授权成功后再发一条不带 action 的更新补全
// from_peer/from_name/session_id/type。两条为独立异步 HTTP，存在到达顺序竞态：
// 若更新先于 new 到达，旧逻辑因查不到记录而丢弃更新，导致最终记录缺失来源名称。
// 改为 upsert 后，无论到达顺序，两条请求都会命中同一条记录并合并各自字段（Updates 只写非零字段，
// 不会用空值覆盖已有信息），彻底消除竞态丢失。
func (as *AuditService) UpsertByPeerIdAndConnId(u *model.AuditConn) error {
	ex := as.InfoByPeerIdAndConnId(u.PeerId, u.ConnId)
	if ex.Id != 0 {
		return DB.Model(&model.AuditConn{}).Where("id = ?", ex.Id).Updates(u).Error
	}
	return DB.Create(u).Error
}

// ConnInfoById
func (as *AuditService) ConnInfoById(id uint) (res *model.AuditConn) {
	res = &model.AuditConn{}
	DB.Where("id = ?", id).First(res)
	return
}

// peerInfo peers 表里与审计展示相关的设备信息。
type peerInfo struct {
	Version  string
	Hostname string
	Alias    string
}

// peerInfos 批量查设备信息：设备 id -> peerInfo。
func peerInfos(ids []string) map[string]peerInfo {
	infoMap := make(map[string]peerInfo, len(ids))
	if len(ids) == 0 {
		return infoMap
	}
	type row struct {
		Id       string
		Version  string
		Hostname string
		Alias    string
	}
	var rows []row
	if err := DB.Model(&model.Peer{}).
		Select("id, version, hostname, alias").
		Where("id in ?", ids).
		Find(&rows).Error; err != nil {
		global.Logger.Warn("peerInfos", err)
		return infoMap
	}
	for _, r := range rows {
		infoMap[r.Id] = peerInfo{Version: r.Version, Hostname: r.Hostname, Alias: r.Alias}
	}
	return infoMap
}

// fillPeerInfo 用 peers 表的值补齐一次审计记录里的版本/主机名/别名；已有值不覆盖。
func fillPeerInfo(peerId string, version, hostname, alias *string, m map[string]peerInfo) {
	if peerId == "" {
		return
	}
	info, ok := m[peerId]
	if !ok {
		return
	}
	if *version == "" {
		*version = info.Version
	}
	if *hostname == "" {
		*hostname = info.Hostname
	}
	if *alias == "" {
		*alias = info.Alias
	}
}

// FillPeerInfoFromPeers 用 peers 表兜底补全「连接日志」的版本 / 主机名 / 别名。
//
// 老客户端不会在 /audit/conn 里上报 version/from_version，主机名和别名本来就由
// 服务端维护；没上报/没存过的字段用 peers 表补齐，保证后台列表和导出都能看到。
// 已存下的值优先——那是「连接发生时」的快照，设备改名后历史记录不会跟着变。
func (as *AuditService) FillPeerInfoFromPeers(c *model.AuditConn) {
	if c == nil {
		return
	}
	needPeer := c.PeerId != "" && (c.Version == "" || c.PeerHostname == "" || c.PeerAlias == "")
	needFrom := c.FromPeer != "" && (c.FromVersion == "" || c.FromHostname == "" || c.FromAlias == "")
	if !needPeer && !needFrom {
		return
	}
	ids := make([]string, 0, 2)
	if needPeer {
		ids = append(ids, c.PeerId)
	}
	if needFrom {
		ids = append(ids, c.FromPeer)
	}
	m := peerInfos(ids)
	if needPeer {
		fillPeerInfo(c.PeerId, &c.Version, &c.PeerHostname, &c.PeerAlias, m)
	}
	if needFrom {
		fillPeerInfo(c.FromPeer, &c.FromVersion, &c.FromHostname, &c.FromAlias, m)
	}
}

// FillFilePeerInfoFromPeers 同 FillPeerInfoFromPeers，用于「文件传输日志」。
func (as *AuditService) FillFilePeerInfoFromPeers(f *model.AuditFile) {
	if f == nil {
		return
	}
	needPeer := f.PeerId != "" && (f.Version == "" || f.PeerHostname == "" || f.PeerAlias == "")
	needFrom := f.FromPeer != "" && (f.FromVersion == "" || f.FromHostname == "" || f.FromAlias == "")
	if !needPeer && !needFrom {
		return
	}
	ids := make([]string, 0, 2)
	if needPeer {
		ids = append(ids, f.PeerId)
	}
	if needFrom {
		ids = append(ids, f.FromPeer)
	}
	m := peerInfos(ids)
	if needPeer {
		fillPeerInfo(f.PeerId, &f.Version, &f.PeerHostname, &f.PeerAlias, m)
	}
	if needFrom {
		fillPeerInfo(f.FromPeer, &f.FromVersion, &f.FromHostname, &f.FromAlias, m)
	}
}

// FillFileListPeerInfo 批量给「文件传输日志」列表补版本/主机名/别名，避免逐条查询。
func (as *AuditService) FillFileListPeerInfo(list []*model.AuditFile) {
	if len(list) == 0 {
		return
	}
	ids := make([]string, 0, len(list)*2)
	for _, f := range list {
		if f == nil {
			continue
		}
		if f.PeerId != "" && (f.Version == "" || f.PeerHostname == "" || f.PeerAlias == "") {
			ids = append(ids, f.PeerId)
		}
		if f.FromPeer != "" && (f.FromVersion == "" || f.FromHostname == "" || f.FromAlias == "") {
			ids = append(ids, f.FromPeer)
		}
	}
	if len(ids) == 0 {
		return
	}
	m := peerInfos(ids)
	for _, f := range list {
		if f == nil {
			continue
		}
		fillPeerInfo(f.PeerId, &f.Version, &f.PeerHostname, &f.PeerAlias, m)
		fillPeerInfo(f.FromPeer, &f.FromVersion, &f.FromHostname, &f.FromAlias, m)
	}
}

// ActiveConnIdByPair 查 (peerId 作为被控端, fromPeer 作为控制端) 的进行中连接 id。
// 没有进行中的会话返回 0。
//
// 用于直连合规执法：设备上报"我想连 target"之后若宽限期内没收到直连上报，
// 需要定位"要断哪条会话"。close_time = 0 表示仍在进行中。
func (as *AuditService) ActiveConnIdByPair(peerId, fromPeer string) int64 {
	if peerId == "" || fromPeer == "" {
		return 0
	}
	res := &model.AuditConn{}
	DB.Where("peer_id = ? and from_peer = ? and close_time = 0", peerId, fromPeer).
		Order("id desc").First(res)
	return res.ConnId
}

// FileInfoById
func (as *AuditService) FileInfoById(id uint) (res *model.AuditFile) {
	res = &model.AuditFile{}
	DB.Where("id = ?", id).First(res)
	return
}

func (as *AuditService) AuditFileList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AuditFileList) {
	res = &model.AuditFileList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AuditFile{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.AuditFiles)
	return
}

// CreateAuditFile
func (as *AuditService) CreateAuditFile(u *model.AuditFile) error {
	res := DB.Create(u).Error
	return res
}
func (as *AuditService) DeleteAuditFile(u *model.AuditFile) error {
	return DB.Delete(u).Error
}

// Update 更新
func (as *AuditService) UpdateAuditFile(u *model.AuditFile) error {
	return DB.Model(u).Updates(u).Error
}

func (as *AuditService) BatchDeleteAuditConn(ids []uint) error {
	return DB.Where("id in (?)", ids).Delete(&model.AuditConn{}).Error
}

// StartStaleConnCloseSweep 后台定时清理“进行中”的孤儿连接审计记录。
// 触发关闭的条件（满足其一即可）：
//  1. 对端设备已离线超过 5 分钟（崩溃 / 网络中断 / 进程被杀死等场景）；
//  2. 记录创建已超过 24 小时（兜底，覆盖控制端升级替换、未收到 close 事件的场景）。
//
// 避免旧客户端退出时未发送 close 审计，导致首页“最近连接记录”永久卡在“进行中”。
// 该操作幂等，多实例部署也安全。
func (as *AuditService) StartStaleConnCloseSweep() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	as.closeStaleConns()
	for range ticker.C {
		as.closeStaleConns()
	}
}

func (as *AuditService) closeStaleConns() {
	now := time.Now()
	offlineThreshold := now.Unix() - 300        // 对端离线超过 5 分钟
	maxAgeThreshold := now.Add(-24 * time.Hour) // 记录超过 24 小时
	sub := DB.Model(&model.Peer{}).
		Select("id").
		Where("last_online_time <= ? OR last_online_time = 0", offlineThreshold)
	res := DB.Model(&model.AuditConn{}).
		Where("close_time = 0").
		Where("(peer_id IN (?)) OR (created_at < ?)", sub, maxAgeThreshold).
		Update("close_time", now.Unix())
	if res.Error != nil {
		global.Logger.Warn("closeStaleConns", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		global.Logger.Infof("closeStaleConns: closed %d stale 'in-progress' audit_conn records", res.RowsAffected)
	}
}

// ========== 连接心跳追踪 ==========
// connHeartbeats 记录每个活跃连接的最近心跳时间。
// key = "peerId:connId", value = time.Time
var connHeartbeats sync.Map

// RecordConnHeartbeat 记录一批连接的心跳时间。
// 在 Heartbeat 控制器中调用，传入客户端心跳上报的活跃连接列表。
func (as *AuditService) RecordConnHeartbeat(peerId string, connIds []int) {
	now := time.Now()
	for _, cid := range connIds {
		connHeartbeats.Store(fmt.Sprintf("%s:%d", peerId, cid), now)
	}
}

// StartConnHeartbeatSweep 后台定时清理心跳超时的连接审计记录。
// 如果某个活跃连接在 60 秒内没有心跳更新（客户端异常断开），则关闭审计记录。
// 与 StartStaleConnCloseSweep 并存：前者更快（60s）、更精细（逐连接）；
// 后者更保守（5min）兜底设备级离线。
func (as *AuditService) StartConnHeartbeatSweep() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	as.closeStaleConnsByHeartbeat()
	for range ticker.C {
		as.closeStaleConnsByHeartbeat()
	}
}

func (as *AuditService) closeStaleConnsByHeartbeat() {
	now := time.Now()
	threshold := now.Add(-60 * time.Second)
	var toClose []string // peerId:connId 格式
	connHeartbeats.Range(func(key, value interface{}) bool {
		if lastBeat, ok := value.(time.Time); ok {
			if lastBeat.Before(threshold) {
				toClose = append(toClose, key.(string))
			}
		}
		return true
	})
	if len(toClose) == 0 {
		return
	}
	// 遍历要关闭的记录
	var closedCount int64
	for _, key := range toClose {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			connHeartbeats.Delete(key)
			continue
		}
		peerId, connIdStr := parts[0], parts[1]
		connId, err := strconv.ParseInt(connIdStr, 10, 64)
		if err != nil {
			connHeartbeats.Delete(key)
			continue
		}
		// 同时检查 peer_id（被控端）和 from_peer（控制端）两个方向：
		// 被控端断开心跳 → 关闭；控制端断开心跳即使被控端还活着，也应关闭。
		res := DB.Model(&model.AuditConn{}).
			Where("close_time = 0").
			Where("(peer_id = ? AND conn_id = ?) OR (from_peer = ? AND conn_id = ?)", peerId, connId, peerId, connId).
			Update("close_time", now.Unix())
		if res.Error == nil {
			closedCount += res.RowsAffected
		}
		connHeartbeats.Delete(key)
	}
	if closedCount > 0 {
		global.Logger.Infof("connHeartbeatSweep: closed %d stale connections (heartbeat timeout)", closedCount)
	}
}

func (as *AuditService) BatchDeleteAuditFile(ids []uint) error {
	return DB.Where("id in (?)", ids).Delete(&model.AuditFile{}).Error
}

// CloseInProgressByFromPeerAndPeer 关闭同一用户(FromPeer)对同一对端(PeerId)仍“进行中”(close_time=0)的连接审计记录。
// 用于解决异常断开（客户端未发送 close 审计）导致“最近连接记录”一直显示“进行中”的问题：
// 当该用户再次向同一对端发起连接(new)时，把上一条仍在进行中的记录置为已关闭，状态即被重置。
// 该操作幂等，多实例部署也安全。
func (as *AuditService) CloseInProgressByFromPeerAndPeer(fromPeer, peerId string) {
	if fromPeer == "" || peerId == "" {
		return
	}
	now := time.Now().Unix()
	res := DB.Model(&model.AuditConn{}).
		Where("close_time = 0 AND from_peer = ? AND peer_id = ?", fromPeer, peerId).
		Update("close_time", now)
	if res.Error != nil {
		global.Logger.Warn("CloseInProgressByFromPeerAndPeer", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		global.Logger.Infof("CloseInProgressByFromPeerAndPeer: closed %d in-progress audit_conn for %s -> %s", res.RowsAffected, fromPeer, peerId)
	}
}
