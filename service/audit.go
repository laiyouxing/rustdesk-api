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

// InfoByPeerIdAndConnId 按 (peer_id, conn_id) 查记录。
//
// conn_id 是客户端「进程内自增」的连接序号，客户端重启后会从 1 重新开始，
// 所以历史记录里同一个 (peer_id, conn_id) 可能有多条。这里优先命中「进行中」
// （close_time = 0）的那条，其次取最新的一条——否则 close 审计会写到很久以前
// 的老记录上，让进行中的记录永远关不掉。
func (as *AuditService) InfoByPeerIdAndConnId(peerId string, connId int64) (res *model.AuditConn) {
	res = &model.AuditConn{}
	if DB.Where("peer_id = ? and conn_id = ? and close_time = 0", peerId, connId).
		Order("id desc").First(res).Error == nil && res.Id != 0 {
		return res
	}
	res = &model.AuditConn{}
	DB.Where("peer_id = ? and conn_id = ?", peerId, connId).Order("id desc").First(res)
	return res
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

// ConnMergeWindow 同一次连接操作的重试合并窗口。
//
// RustDesk 建立连接时会多次尝试（直连打洞失败回退中继、网络抖动后重连、并行拨号），
// 每一次尝试都会被控端各发一条 new 审计，后台看起来就是「同一个用户点了一次，
// 却出现三四条连接记录」。同一对 (控制端 from_peer → 被控端 peer_id) 在窗口内
// 再次鉴权成功，就认为仍是同一次操作，合并进同一条记录。
const ConnMergeWindow = 120 * time.Second

// MergeConnAttempt 把本次「鉴权成功」的连接并入同一次操作已有的记录。
//
// 返回被合并记录的主键；返回 0 表示没有可合并的记录，调用方按原有逻辑 upsert。
//
// 合并方式：保留最早那条（创建时间即这次操作的开始时间），把 conn_id / session_id
// 等换成最新值——conn_id 必须换，后续 close 审计是靠 (peer_id, conn_id) 定位记录的；
// 同时删掉本次尝试临时建的行，以及同一次操作里没能登录成功的尝试行（这些行 from_peer
// 为空，只能靠「同被控端 + 同控制端 IP + 窗口内」识别）。
func (as *AuditService) MergeConnAttempt(cur *model.AuditConn) uint {
	if cur == nil || cur.PeerId == "" || cur.FromPeer == "" {
		return 0
	}
	now := time.Now()
	// 本次尝试通常已由 new 事件建过记录；若 authed 先到（日志里出现过竞态）则还没有。
	curRow := &model.AuditConn{}
	hasCur := DB.Where("peer_id = ? and conn_id = ?", cur.PeerId, cur.ConnId).
		Order("id desc").First(curRow).Error == nil && curRow.Id != 0
	// 这条记录本身已经是「已鉴权、进行中」的会话行（重复上报的同一条 authed 事件）：
	// 直接算作已处理，避免把重复事件当成新尝试去合并到别的记录上。
	if hasCur && curRow.FromPeer == cur.FromPeer && curRow.CloseTime == 0 {
		return curRow.Id
	}

	q := DB.Model(&model.AuditConn{}).
		Where("peer_id = ? and from_peer = ?", cur.PeerId, cur.FromPeer).
		Where("(close_time = 0 OR close_time >= ?)", now.Add(-ConnMergeWindow).Unix())
	if hasCur {
		q = q.Where("id <> ?", curRow.Id)
	}
	primary := &model.AuditConn{}
	if q.Order("id desc").First(primary).Error != nil || primary.Id == 0 {
		return 0
	}

	updates := map[string]interface{}{
		"conn_id":       cur.ConnId,
		"close_time":    0, // 同一次操作的重试，记录回到「进行中」
		"attempt_count": gorm.Expr("attempt_count + 1"),
	}
	if cur.SessionId != "" && cur.SessionId != "0" {
		updates["session_id"] = cur.SessionId
	}
	if cur.Type != 0 {
		updates["type"] = cur.Type
	}
	if cur.Ip != "" {
		updates["ip"] = cur.Ip
	}
	for k, v := range map[string]string{
		"version":       cur.Version,
		"from_version":  cur.FromVersion,
		"from_name":     cur.FromName,
		"peer_hostname": cur.PeerHostname,
		"peer_alias":    cur.PeerAlias,
		"from_hostname": cur.FromHostname,
		"from_alias":    cur.FromAlias,
	} {
		if v != "" {
			updates[k] = v
		}
	}
	if err := DB.Model(&model.AuditConn{}).Where("id = ?", primary.Id).Updates(updates).Error; err != nil {
		global.Logger.Warn("MergeConnAttempt update", err)
		return 0
	}

	// 本次尝试的临时行：信息已经搬到主记录上了。
	if hasCur && curRow.Id != primary.Id {
		if err := DB.Delete(&model.AuditConn{}, curRow.Id).Error; err != nil {
			global.Logger.Warn("MergeConnAttempt delete current", err)
		}
	}
	// 同一次操作里没登录成功的尝试：没有 from_peer，靠同 IP + 时间窗归属。
	if cur.Ip != "" {
		DB.Where("peer_id = ? and from_peer = '' and ip = ? and id <> ? and created_at >= ?",
			cur.PeerId, cur.Ip, primary.Id, now.Add(-ConnMergeWindow)).
			Delete(&model.AuditConn{})
	}
	global.Logger.Infof("MergeConnAttempt: conn %d merged into audit id %d (%s -> %s)",
		cur.ConnId, primary.Id, cur.FromPeer, cur.PeerId)
	return primary.Id
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
	// 从没登录成功的尝试（from_peer 为空，只有 new 没有补全）10 分钟后视为失败连接，关掉。
	// 这类行是客户端重试/并行拨号的残留，不该在「进行中」里一直挂着。
	resFailed := DB.Model(&model.AuditConn{}).
		Where("close_time = 0 AND from_peer = '' AND created_at < ?", now.Add(-10*time.Minute)).
		Update("close_time", now.Unix())
	if resFailed.Error != nil {
		global.Logger.Warn("closeStaleConns failed-attempts", resFailed.Error)
	} else if resFailed.RowsAffected > 0 {
		global.Logger.Infof("closeStaleConns: closed %d unauthenticated 'in-progress' audit_conn records", resFailed.RowsAffected)
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

// CloseInProgressByFromPeerAndPeerExcept 同 CloseInProgressByFromPeerAndPeer，但保留本次连接
// （keepConnId）的记录不关闭。
//
// 为什么需要：鉴权成功后的补全更新会先把本次记录 upsert 成「进行中」，随后再关旧记录时
// 若不加排除，会把刚建立的这条一起关掉——表现就是连接明明还在，日志却已经显示断开。
func (as *AuditService) CloseInProgressByFromPeerAndPeerExcept(fromPeer, peerId string, keepConnId int64) {
	if fromPeer == "" || peerId == "" {
		return
	}
	now := time.Now().Unix()
	res := DB.Model(&model.AuditConn{}).
		Where("close_time = 0 AND from_peer = ? AND peer_id = ? AND conn_id <> ?", fromPeer, peerId, keepConnId).
		Update("close_time", now)
	if res.Error != nil {
		global.Logger.Warn("CloseInProgressByFromPeerAndPeerExcept", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		global.Logger.Infof("CloseInProgressByFromPeerAndPeerExcept: closed %d stale audit_conn for %s -> %s", res.RowsAffected, fromPeer, peerId)
	}
}
