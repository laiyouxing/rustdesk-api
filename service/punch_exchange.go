package service

import (
	"sync"
	"time"
)

// PunchAddrTTL 打洞地址有效期（秒）。超过即视为设备已离线/放弃连接。
const PunchAddrTTL = 60

// PunchAddr 一个可打洞的地址
type PunchAddr struct {
	Addr     string
	AddrType string
	Delta    int32
	SyncAtMs int64
}

// PunchEntry 一次上报
type PunchEntry struct {
	PeerId    string
	Target    string
	Addrs     []PunchAddr
	UpdatedAt int64
}

// PunchExchangeService 打洞地址交换服务。
//
// 设计目的：让 UDP 打洞在**建立中继之前**完成——双方通过 api-server
// 交换 STUN 探测到的公网地址，控制端拿到地址后立即打洞，成功则直接在
// 直连上走完整握手（secure_connection + Login），不再需要"先中继、
// 再打洞、再换流"，从根本上消除双连接与换流竞态。
//
// 纯内存存储 + TTL，不落库：
//   - byPeer: 设备自己上报的地址（控制端拉取对端地址用）
//   - inbox:  "谁想连我"（被控端周期拉取，发现连接请求后主动打洞）
type PunchExchangeService struct {
	mu     sync.RWMutex
	byPeer map[string]*PunchEntry
	inbox  map[string]map[string]*PunchEntry
}

func NewPunchExchangeService() *PunchExchangeService {
	s := &PunchExchangeService{
		byPeer: make(map[string]*PunchEntry),
		inbox:  make(map[string]map[string]*PunchEntry),
	}
	go s.cleanupLoop()
	return s
}

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
		s.mu.Unlock()
	}
}

// Upsert 上报。peerId 为上报者；target 非空表示"我想连 target"。
func (s *PunchExchangeService) Upsert(peerId, target string, addrs []PunchAddr) {
	if peerId == "" || len(addrs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := &PunchEntry{
		PeerId:    peerId,
		Target:    target,
		Addrs:     addrs,
		UpdatedAt: time.Now().Unix(),
	}
	s.byPeer[peerId] = entry
	if target != "" && target != peerId {
		if s.inbox[target] == nil {
			s.inbox[target] = make(map[string]*PunchEntry)
		}
		s.inbox[target][peerId] = entry
	}
}

// GetByPeer 拉取某设备自己上报的地址（控制端用：peer_id = 被控端 id）
func (s *PunchExchangeService) GetByPeer(peerId string) []PunchAddr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.byPeer[peerId]; ok {
		return e.Addrs
	}
	return nil
}

// GetInbox 拉取"想连我的设备"列表（被控端用：peer_id = 自己）
func (s *PunchExchangeService) GetInbox(peerId string) []*PunchEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*PunchEntry, 0, len(s.inbox[peerId]))
	for _, e := range s.inbox[peerId] {
		out = append(out, e)
	}
	return out
}
