package api

import (
	"github.com/gin-gonic/gin"
	requstform "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// Punch 打洞地址交换（走 api-server，不依赖中继连接）
//
// 背景：UDP 打洞需要双方事先知道彼此的公网地址。原来通过已建立的中继
// 连接交换（PunchPeerAddr），导致必须先建中继、再打洞、再换流，出现
// 双连接与切换竞态。改为通过 api-server 交换后，打洞可以在中继之前
// 完成，连接建立时直接使用直连。
type Punch struct{}

// Report 上报自己的打洞地址（STUN 探测结果）
// @Tags 打洞地址交换
// @Summary 上报打洞地址
// @Description 设备周期性上报自己的公网打洞地址；控制端连接时 target 填被控端 id，被控端据此发现连接请求
// @Accept json
// @Produce json
// @Param body body api.PunchAddrForm true "上报内容"
// @Success 200 {object} response.Response
// @Router /punch/addr [post]
func (p *Punch) Report(c *gin.Context) {
	f := &requstform.PunchAddrForm{}
	if err := c.ShouldBindJSON(f); err != nil || f.PeerId == "" || len(f.Addrs) == 0 {
		response.Fail(c, 101, "参数错误")
		return
	}
	addrs := make([]service.PunchAddr, 0, len(f.Addrs))
	for _, a := range f.Addrs {
		if a.Addr == "" {
			continue
		}
		addrs = append(addrs, service.PunchAddr{
			Addr:     a.Addr,
			AddrType: a.AddrType,
			Delta:    a.Delta,
			SyncAtMs: a.SyncAtMs,
		})
	}
	if len(addrs) == 0 {
		response.Fail(c, 101, "无有效地址")
		return
	}
	service.AllService.PunchExchangeService.Upsert(f.PeerId, f.Target, addrs)
	response.Success(c, gin.H{"ok": true})
}

// Get 拉取某设备上报的打洞地址（控制端用：peer_id = 被控端 id）
// @Tags 打洞地址交换
// @Summary 拉取对端打洞地址
// @Produce json
// @Param peer_id query string true "对端设备 peer id"
// @Success 200 {object} response.Response
// @Router /punch/addr [get]
func (p *Punch) Get(c *gin.Context) {
	peerId := c.Query("peer_id")
	if peerId == "" {
		response.Fail(c, 101, "peer_id 不能为空")
		return
	}
	addrs := service.AllService.PunchExchangeService.GetByPeer(peerId)
	out := make([]gin.H, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, gin.H{
			"addr":       a.Addr,
			"addr_type":  a.AddrType,
			"delta":      a.Delta,
			"sync_at_ms": a.SyncAtMs,
		})
	}
	response.Success(c, gin.H{"addrs": out})
}

// Inbox 拉取"想连我的设备"列表（被控端用：peer_id = 自己）
// @Tags 打洞地址交换
// @Summary 拉取待处理的连接请求地址
// @Produce json
// @Param peer_id query string true "本机 peer id"
// @Success 200 {object} response.Response
// @Router /punch/addr/inbox [get]
func (p *Punch) Inbox(c *gin.Context) {
	peerId := c.Query("peer_id")
	if peerId == "" {
		response.Fail(c, 101, "peer_id 不能为空")
		return
	}
	entries := service.AllService.PunchExchangeService.GetInbox(peerId)
	out := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		addrs := make([]gin.H, 0, len(e.Addrs))
		for _, a := range e.Addrs {
			addrs = append(addrs, gin.H{
				"addr":       a.Addr,
				"addr_type":  a.AddrType,
				"delta":      a.Delta,
				"sync_at_ms": a.SyncAtMs,
			})
		}
		out = append(out, gin.H{"peer_id": e.PeerId, "addrs": addrs})
	}
	response.Success(c, gin.H{"requests": out})
}
