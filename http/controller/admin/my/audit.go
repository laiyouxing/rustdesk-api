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
	// Enrich with peer hostname/alias and close_time string
	type ConnEntry struct {
		model.AuditConn
		PeerHostname string `json:"peer_hostname"`
		PeerAlias    string `json:"peer_alias"`
		CloseTimeStr string `json:"close_time_str"`
	}
	var enriched []*ConnEntry
	var targetPeerIds []string
	for _, conn := range res.AuditConns {
		if conn.PeerId != "" {
			targetPeerIds = append(targetPeerIds, conn.PeerId)
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
		// 版本优先用连接发生时记录的值；老记录没存过，用 peers 表现值兜底。
		if conn.Version == "" {
			conn.Version = h.Version
		}
		closeTimeStr := ""
		if conn.CloseTime > 0 {
			closeTimeStr = time.Unix(conn.CloseTime, 0).Format("2006-01-02 15:04:05")
		}
		enriched = append(enriched, &ConnEntry{
			AuditConn:    *conn,
			PeerHostname: h.Hostname,
			PeerAlias:    h.Alias,
			CloseTimeStr: closeTimeStr,
		})
	}
	if enriched == nil {
		enriched = []*ConnEntry{}
	}
	response.Success(c, gin.H{"list": enriched, "total": res.Total})
}
