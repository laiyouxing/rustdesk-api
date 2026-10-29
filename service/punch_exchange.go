package service

import (
	"strings"
	"sync"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

// PunchAddrTTL 打洞地址有效期（秒）。超过即视为设备已离线/放弃连接。
const PunchAddrTTL = 60

// PunchDirectGrace 直连上报宽限期（秒）。
// 客户端直连建立后必须在此时限内把两端网络标识上报上来；超时未收到两端
// 上报、且该对端没有活跃的中继会话，则判定为"未登记的直连"，下发
// disconnect 强制断开该会话。
const PunchDirectGrace = 30

// PunchDirectOkTTL 直连上报记录有效期（秒）。
const PunchDirectOkTTL = 300

// DirectOkEntry 一条直连成功上报（合规审计用）。
type DirectOkEntry struct {
	PeerId     string
	Target     string
	ConnType   string // direct_v4 / direct_v6
	PublicAddr string
	LanAddrs   []string
	MacHashes  []string
	UpdatedAt  int64
}

// directPending 一次"打算直连"的登记，由带 target 的地址上报产生。
// 必须与 byPeer 分开存：设备周期性上报自己地址时 target 为空，
// 若复用 byPeer 会把意图覆盖掉，巡检就永远看不到。
type directPending struct {
	Target    string
	CreatedAt int64
}

// 地址来源（PunchAddr.Kind）
const (
	PunchKindLastSuccess = "last_success" // 上一会话成功用过的映射
	PunchKindStunPrimary = "stun_primary" // 主 STUN 映射
	PunchKindStunAlt     = "stun_alt"     // 备 STUN 映射（对称 NAT 下不同）
	PunchKindUpnp        = "upnp"         // UPnP 外部映射
	PunchKindLan         = "lan"          // 内网网卡地址
	PunchKindV6          = "v6"           // 公网 IPv6
	PunchKindTcp         = "tcp"          // TCP 同步打开端口
)

// PunchAddr 一个可打洞的地址
type PunchAddr struct {
	Addr     string
	AddrType string
	Kind     string
	Priority int32
	Delta    int32
	SyncAtMs int64
}

// PunchMeta 设备网络环境
type PunchMeta struct {
	NatType  string
	Upnp     bool
	ViaProxy bool
}

// PunchEntry 一次上报
type PunchEntry struct {
	PeerId    string
	Target    string
	Meta      PunchMeta
	Addrs     []PunchAddr
	UpdatedAt int64
}

// PunchExchangeService 打洞地址交换服务。
//
// 设计目的：让 UDP 打洞在**建立中继之前**完成——双方通过 api-server
// 交换各自探测到的全部候选地址（多 STUN 映射 / UPnP / LAN / IPv6 / TCP /
// 上一会话成功地址）与网络环境（NAT 类型、是否代理），控制端拿到地址后
// 立即打洞，成功则直接在直连上走完整握手，不再需要"先中继、再打洞、
// 再换流"，从根本上消除双连接与换流竞态。
//
// 纯内存存储 + TTL，不落库：
//   - byPeer: 设备自己上报的地址（控制端拉取对端地址用）
//   - inbox:  "谁想连我"（被控端周期拉取，发现连接请求后主动打洞）
type PunchExchangeService struct {
	mu     sync.RWMutex
	byPeer map[string]*PunchEntry
	inbox  map[string]map[string]*PunchEntry
	// 直连合规上报与执法状态（见文件末尾）
	directOk    map[string]*DirectOkEntry // key: "A|B"
	pending     map[string]directPending  // key: "A|B"，一次直连意图
	strikes     map[string]int            // key: "A|B"，连续确认"无上报"的轮数
	disconnects map[string][]int          // peerId -> 待下发的 connID 列表
}

func NewPunchExchangeService() *PunchExchangeService {
	s := &PunchExchangeService{
		byPeer:      make(map[string]*PunchEntry),
		inbox:       make(map[string]map[string]*PunchEntry),
		directOk:    make(map[string]*DirectOkEntry),
		pending:     make(map[string]directPending),
		strikes:     make(map[string]int),
		disconnects: make(map[string][]int),
	}
	go s.cleanupLoop()
	return s
}

func pairKey(a, b string) string { return a + "|" + b }

func (s *PunchExchangeService) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now().Unix()
		for k, v := range s.byPeer {
			if now-v.UpdatedAt > PunchAddrTTL {
				delete(s.byPeer, k)
			}
		}
		for t, m := range s.inbox {
			for k, v := range m {
				if now-v.UpdatedAt > PunchAddrTTL {
					delete(m, k)
				}
			}
			if len(m) == 0 {
				delete(s.inbox, t)
			}
		}
		for k, v := range s.directOk {
			if now-v.UpdatedAt > PunchDirectOkTTL {
				delete(s.directOk, k)
			}
		}
		// 收集需要执法的意图（在锁内快照，锁外查库，避免持锁做 DB IO）
		type suspect struct {
			host, guest string
			key         string
		}
		var suspects []suspect
		for k, p := range s.pending {
			if now-p.CreatedAt <= PunchDirectGrace {
				continue // 宽限期内，等上报
			}
			host, guest := splitPair(k)
			// 任一方向有上报（直连成功 direct_v4/direct_v6，或回退中继 relay）
			// 即视为已登记，清理意图。
			//
			// 为什么"回退中继"也算合规：打洞失败回退中继是正常路径，此时不会
			// 有直连上报。若不认这条，服务端会把正常的中继会话误判成
			// "未登记的直连"而强制断开。
			if s.hasOutcomeLocked(host, guest) || s.hasOutcomeLocked(guest, host) {
				delete(s.pending, k)
				continue
			}
			suspects = append(suspects, suspect{host: host, guest: guest, key: k})
		}
		s.mu.Unlock()

		// 锁外判定（要查库，不能持锁）。
		//
		// 二次确认（strikes >= 2）再断开：客户端的上报是 fire-and-forget，
		// 网络抖动时可能整体丢失一次。若一轮没看到上报就断，会把正常的
		// 中继回退会话误杀。要求连续两轮（约 10s 间隔）都确认"无任何上报"
		// 才动手，足以过滤瞬时丢包。
		for _, sp := range suspects {
			connId := AllService.AuditService.ActiveConnIdByPair(sp.host, sp.guest)
			if connId == 0 {
				connId = AllService.AuditService.ActiveConnIdByPair(sp.guest, sp.host)
			}
			if connId == 0 {
				// 没有活跃会话：打洞失败且未回退/已结束，不必断开。
				// 意图不清，交给 TTL 自然过期。
				s.mu.Lock()
				delete(s.strikes, sp.key)
				s.mu.Unlock()
				continue
			}
			s.mu.Lock()
			s.strikes[sp.key]++
			strikes := s.strikes[sp.key]
			if strikes < 2 {
				s.mu.Unlock()
				Logger.Infof(
					"direct-ok missing between %s and %s, strike %d/%d (grace %ds)",
					sp.host, sp.guest, strikes, 2, PunchDirectGrace,
				)
				continue
			}
			// 只下发给 host：connId 取自 host 侧的审计记录，对 guest 侧
			// 的 conn id 无意义，错发可能断掉 guest 的其它会话。
			// 断开 host 一条即可，guest 会收到对端关闭。
			s.disconnects[sp.host] = append(s.disconnects[sp.host], int(connId))
			delete(s.pending, sp.key)
			delete(s.strikes, sp.key)
			s.mu.Unlock()
			Logger.Infof(
				"direct-ok missing between %s and %s (grace %ds), forcing disconnect conn=%d",
				sp.host, sp.guest, PunchDirectGrace, connId,
			)
		}
	}
}

func splitPair(k string) (host, guest string) {
	for i := 0; i < len(k); i++ {
		if k[i] == '|' {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}

// RecordDirectOk 记录一次直连成功上报。
//
// 同时做两件事：
//  1. 写内存（供执法巡检"是否已登记"判定，TTL 短）；
//  2. 落库（供后台审计页长期追溯，这是合规留痕的主要价值）。
func (s *PunchExchangeService) RecordDirectOk(e *DirectOkEntry) {
	if e.PeerId == "" || e.Target == "" {
		return
	}
	e.UpdatedAt = time.Now().Unix()
	s.mu.Lock()
	s.directOk[pairKey(e.PeerId, e.Target)] = e
	s.mu.Unlock() // 先放锁再写库：不在持锁状态下做 DB IO

	rec := &model.PunchDirectOk{
		PeerId:     e.PeerId,
		Target:     e.Target,
		ConnType:   e.ConnType,
		PublicAddr: e.PublicAddr,
		LanAddrs:   strings.Join(e.LanAddrs, ","),
		MacHashes:  strings.Join(e.MacHashes, ","),
	}
	if err := DB.Create(rec).Error; err != nil {
		// 落库失败不影响执法判定（内存里已有），只记日志
		Logger.Errorf("RecordDirectOk persist err: %v", err)
	}
}

// DirectOkList 后台审计页：直连上报记录分页列表。
func (s *PunchExchangeService) DirectOkList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.PunchDirectOkList) {
	res = &model.PunchDirectOkList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.PunchDirectOk{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.PunchDirectOks)
	return
}

func (s *PunchExchangeService) hasOutcomeLocked(peerId, target string) bool {
	_, ok := s.directOk[pairKey(peerId, target)]
	return ok
}

// PopDisconnects 取出并清空某设备待下发的断开指令（由心跳带出）。
func (s *PunchExchangeService) PopDisconnects(peerId string) []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.disconnects[peerId]
	delete(s.disconnects, peerId)
	return ids
}

// Upsert 上报。peerId 为上报者；target 非空表示"我想连 target"。
func (s *PunchExchangeService) Upsert(peerId, target string, meta PunchMeta, addrs []PunchAddr) {
	if peerId == "" || len(addrs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := &PunchEntry{
		PeerId:    peerId,
		Target:    target,
		Meta:      meta,
		Addrs:     addrs,
		UpdatedAt: time.Now().Unix(),
	}
	s.byPeer[peerId] = entry
	if target != "" && target != peerId {
		if s.inbox[target] == nil {
			s.inbox[target] = make(map[string]*PunchEntry)
		}
		s.inbox[target][peerId] = entry
		// 记录一次"打算直连"的意图，供宽限期后校验直连上报是否到位。
		// 已存在同 pair 的意图时只刷新时间，不重置——否则控制端重连时
		// 会不断把宽限期往后推，永远等不到执法。
		k := pairKey(peerId, target)
		if _, ok := s.pending[k]; !ok {
			s.pending[k] = directPending{Target: target, CreatedAt: time.Now().Unix()}
		}
	}
}

// GetByPeer 拉取某设备自己上报的地址与网络环境（控制端用：peer_id = 被控端 id）
func (s *PunchExchangeService) GetByPeer(peerId string) (*PunchEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.byPeer[peerId]; ok {
		// 返回浅拷贝，避免调用方持有锁期间的数据被后续覆盖
		cp := *e
		cp.Addrs = append([]PunchAddr(nil), e.Addrs...)
		return &cp, true
	}
	return nil, false
}

// GetInbox 拉取"想连我的设备"列表（被控端用：peer_id = 自己）
func (s *PunchExchangeService) GetInbox(peerId string) []*PunchEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*PunchEntry, 0, len(s.inbox[peerId]))
	for _, e := range s.inbox[peerId] {
		cp := *e
		cp.Addrs = append([]PunchAddr(nil), e.Addrs...)
		out = append(out, &cp)
	}
	return out
}
