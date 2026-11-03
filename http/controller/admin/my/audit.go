package my

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
	"time"
)

type Audit struct {
}

// List 普通用户的远程连接记录列表
func (a *Audit) List(c *gin.Context) {
	query := &admin.AuditQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	res := service.AllService.AuditService.AuditConnList(query.Page, query.PageSize, func(tx *gorm.DB) {
		var peerIds []string
		service.DB.Model(&model.Peer{}).Where("user_id = ?", u.Id).Pluck("id", &peerIds)
		if len(peerIds) > 0 {
			tx.Where("from_peer in ?", peerIds)
		} else {
			tx.Where("1 = 0")
		}
		tx.Where("action = 'new'")
		tx.Order("id desc")
	})
	// Enrich：主机名/别名/版本来自 peers 表；记录里已存的（写入时快照）优先。
	type ConnEntry struct {
		model.AuditConn
		CloseTimeStr string `json:"close_time_str"`
	}
	var enriched []*ConnEntry
	var targetPeerIds []string
	for _, conn := range res.AuditConns {
		if conn.PeerId != "" {
			targetPeerIds = append(targetPeerIds, conn.PeerId)
		}
		if conn.FromPeer != "" {
			targetPeerIds = append(targetPeerIds, conn.FromPeer)
		}
	}
	type peerMeta struct{ Hostname, Alias, Version string }
	hostMap := make(map[string]peerMeta)
	if len(targetPeerIds) > 0 {
		type Pa struct {
			Id       string
			Hostname string
			Alias    string
			Version  string
		}
		var pa []Pa
		service.DB.Model(&model.Peer{}).Select("id, hostname, alias, version").Where("id in ?", targetPeerIds).Find(&pa)
		for _, p := range pa {
			hostMap[p.Id] = peerMeta{p.Hostname, p.Alias, p.Version}
		}
	}
	for _, conn := range res.AuditConns {
		h := hostMap[conn.PeerId]
		fh := hostMap[conn.FromPeer]
		// 版本/主机名/别名优先用连接发生时记录的值；老记录没存过，用 peers 表现值兜底。
		if conn.Version == "" {
			conn.Version = h.Version
		}
		if conn.PeerHostname == "" {
			conn.PeerHostname = h.Hostname
		}
		if conn.PeerAlias == "" {
			conn.PeerAlias = h.Alias
		}
		if conn.FromVersion == "" {
			conn.FromVersion = fh.Version
		}
		if conn.FromHostname == "" {
			conn.FromHostname = fh.Hostname
		}
		if conn.FromAlias == "" {
			conn.FromAlias = fh.Alias
		}
		closeTimeStr := ""
		if conn.CloseTime > 0 {
			closeTimeStr = time.Unix(conn.CloseTime, 0).Format("2006-01-02 15:04:05")
		}
		enriched = append(enriched, &ConnEntry{
			AuditConn:    *conn,
			CloseTimeStr: closeTimeStr,
		})
	}
	if enriched == nil {
		enriched = []*ConnEntry{}
	}
	response.Success(c, gin.H{"list": enriched, "total": res.Total})
}
