package api

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	request "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"time"
)

type Audit struct {
}

// AuditConn
// @Tags 审计
// @Summary 审计连接
// @Description 审计连接
// @Accept  json
// @Produce  json
// @Param body body api.AuditConnForm true "审计连接"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/conn [post]
func (a *Audit) AuditConn(c *gin.Context) {
	af := &request.AuditConnForm{}
	err := c.ShouldBindBodyWith(af, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	/*ttt := &gin.H{}
	c.ShouldBindBodyWith(ttt, binding.JSON)
	fmt.Println(ttt)*/
	ac := af.ToAuditConn()
	// 版本 / 主机名 / 别名补全：新版客户端会带 version/from_version，老客户端不带；
	// 主机名与别名由服务端 peers 表维护。统一在这里补齐，写库时即成为快照。
	service.AllService.AuditService.FillPeerInfoFromPeers(ac)
	if af.Action == model.AuditActionNew {
		// 同一用户再次向同一对端发起连接时，先把上一条仍“进行中”的记录关闭，
		// 避免异常断开（未收到 close 审计）导致“最近连接记录”一直显示“进行中”。
		service.AllService.AuditService.CloseInProgressByFromPeerAndPeer(ac.FromPeer, ac.PeerId)
		// new 请求不携带 session_id，ToAuditConn 会把它格式化成 "0"；置空避免 upsert 时
		// 用 "0" 覆盖后续（或已先到）更新请求写入的真实 session_id。
		ac.SessionId = ""
		// 用 upsert 而非直接 Create：若不带 action 的更新请求先到达并已创建记录，
		// 这里应命中同一条记录补齐 ip/action，而不是再插一条重复记录。
		service.AllService.AuditService.UpsertByPeerIdAndConnId(ac)
	} else if af.Action == model.AuditActionClose {
		ex := service.AllService.AuditService.InfoByPeerIdAndConnId(af.Id, af.ConnId)
		if ex.Id != 0 {
			ex.CloseTime = time.Now().Unix()
			service.AllService.AuditService.UpdateAuditConn(ex)
		}
	} else if af.Action == "" {
		// 授权成功：先尝试把这次尝试并入「同一次操作」的已有记录。
		// 网络抖动重连、NAT 打洞重试都会走到这里——不合并的话，用户点一次连接
		// 会在后台留下三四条记录。
		if merged := service.AllService.AuditService.MergeConnAttempt(ac); merged != 0 {
			// 已并入既有记录（conn_id 等已换成本次的值），不再单独建行。
			response.Success(c, "")
			return
		}
		up := &model.AuditConn{
			Action:       model.AuditActionNew,
			ConnId:       ac.ConnId,
			PeerId:       ac.PeerId,
			FromPeer:     ac.FromPeer,
			FromName:     ac.FromName,
			SessionId:    ac.SessionId,
			Type:         ac.Type,
			Version:      ac.Version,
			FromVersion:  ac.FromVersion,
			PeerHostname: ac.PeerHostname,
			PeerAlias:    ac.PeerAlias,
			FromHostname: ac.FromHostname,
			FromAlias:    ac.FromAlias,
			OpId:         ac.OpId,
		}
		// 先关掉同 (from_peer, peer_id) 更早的「进行中」记录，再写本次记录。
		// 顺序很重要：反过来的话会把刚建立的这条一起关掉（见函数注释）。
		if ac.FromPeer != "" && ac.PeerId != "" {
			service.AllService.AuditService.CloseInProgressByFromPeerAndPeerExcept(ac.FromPeer, ac.PeerId, ac.ConnId)
		}
		service.AllService.AuditService.UpsertByPeerIdAndConnId(up)
	}
	response.Success(c, "")
}

// AuditFile
// @Tags 审计
// @Summary 审计文件
// @Description 审计文件
// @Accept  json
// @Produce  json
// @Param body body api.AuditFileForm true "审计文件"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/file [post]
func (a *Audit) AuditFile(c *gin.Context) {
	aff := &request.AuditFileForm{}
	err := c.ShouldBindBodyWith(aff, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	//ttt := &gin.H{}
	//c.ShouldBindBodyWith(ttt, binding.JSON)
	//fmt.Println(ttt)
	af := aff.ToAuditFile()
	// 版本 / 主机名 / 别名补全：老客户端不上报这些，用 peers 表兜底。
	service.AllService.AuditService.FillFilePeerInfoFromPeers(af)
	service.AllService.AuditService.CreateAuditFile(af)
	response.Success(c, "")
}
