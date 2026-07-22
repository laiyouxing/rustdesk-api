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
		// 授权成功后的补全更新：改为 upsert，若 new 请求尚未落库（竞态）也能先建记录，
		// 避免来源名称等信息因查不到记录而丢失。
		up := &model.AuditConn{
			ConnId:    ac.ConnId,
			PeerId:    ac.PeerId,
			FromPeer:  ac.FromPeer,
			FromName:  ac.FromName,
			SessionId: ac.SessionId,
			Type:      ac.Type,
		}
		service.AllService.AuditService.UpsertByPeerIdAndConnId(up)
		// 补全更新携带了完整来源信息时，也尝试关闭同 (from_peer, peer_id) 的旧进行中记录。
		// 解决重连时 "new" 请求缺少 peer 字段导致旧记录未关闭的问题。
		if ac.FromPeer != "" && ac.PeerId != "" {
			service.AllService.AuditService.CloseInProgressByFromPeerAndPeer(ac.FromPeer, ac.PeerId)
		}
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
	service.AllService.AuditService.CreateAuditFile(af)
	response.Success(c, "")
}
