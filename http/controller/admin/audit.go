package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
	"time"
)

type Audit struct {
}

// ConnList 列表
// @Tags 链接日志
// @Summary 链接日志列表
// @Description 链接日志列表
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Param peer_id query int false "目标设备"
// @Param from_peer query int false "来源设备"
// @Success 200 {object} response.Response{data=model.AuditConnList}
// @Failure 500 {object} response.Response
// @Router /admin/audit_conn/list [get]
// @Security token
func (a *Audit) ConnList(c *gin.Context) {
	query := &admin.AuditQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.AuditService.AuditConnList(query.Page, query.PageSize, func(tx *gorm.DB) {
		admin.ApplyAuditCommon(tx, query)
		// action：留空时默认只看 new（保持原有行为）；
		// 显式传值则可查 close 等其他动作。
		if query.Action != "" {
			tx.Where("action = ?", query.Action)
		} else {
			tx.Where("action = 'new'")
		}
		admin.ApplyAuditOrder(tx, query)
	})
	// Enrich：主机名/别名/版本来自 peers 表；记录里已存的（写入时快照）优先。
	type ConnEntry struct {
		model.AuditConn
		CloseTimeStr string `json:"close_time_str"`
	}
	var enriched []*ConnEntry
	var peerIds []string
	for _, conn := range res.AuditConns {
		if conn.PeerId != "" {
			peerIds = append(peerIds, conn.PeerId)
		}
		if conn.FromPeer != "" {
			peerIds = append(peerIds, conn.FromPeer)
		}
	}
	type peerMeta struct{ Hostname, Alias, Version string }
	hostMap := make(map[string]peerMeta)
	if len(peerIds) > 0 {
		type Pa struct {
			Id       string
			Hostname string
			Alias    string
			Version  string
		}
		var pa []Pa
		service.DB.Model(&model.Peer{}).Select("id, hostname, alias, version").Where("id in ?", peerIds).Find(&pa)
		for _, p := range pa {
			hostMap[p.Id] = peerMeta{p.Hostname, p.Alias, p.Version}
		}
	}
	for _, conn := range res.AuditConns {
		h := hostMap[conn.PeerId]
		fh := hostMap[conn.FromPeer]
		// 版本/主机名/别名优先用连接发生时记录的值；老记录没存过，再用 peers 表的
		// 现值兜底，这样升级前的历史数据也能显示这些信息。
		if conn.Version == "" {
			conn.Version = h.Version
		}
		if conn.FromVersion == "" {
			conn.FromVersion = fh.Version
		}
		if conn.PeerHostname == "" {
			conn.PeerHostname = h.Hostname
		}
		if conn.PeerAlias == "" {
			conn.PeerAlias = h.Alias
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

// ConnDelete 删除
// @Tags 链接日志
// @Summary 链接日志删除
// @Description 链接日志删除
// @Accept  json
// @Produce  json
// @Param body body model.AuditConn true "链接日志信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/audit_conn/delete [post]
// @Security token
func (a *Audit) ConnDelete(c *gin.Context) {
	f := &model.AuditConn{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id := f.Id
	errList := global.Validator.ValidVar(c, id, "required,gt=0")
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	l := service.AllService.AuditService.ConnInfoById(f.Id)
	if l.Id > 0 {
		err := service.AllService.AuditService.DeleteAuditConn(l)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, err.Error())
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// BatchConnDelete 删除
// @Tags 链接日志
// @Summary 链接日志批量删除
// @Description 链接日志批量删除
// @Accept  json
// @Produce  json
// @Param body body admin.AuditConnLogIds true "链接日志"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/audit_conn/batchDelete [post]
// @Security token
func (a *Audit) BatchConnDelete(c *gin.Context) {
	f := &admin.AuditConnLogIds{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(f.Ids) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	err := service.AllService.AuditService.BatchDeleteAuditConn(f.Ids)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
}

// FileList 列表
// @Tags 文件日志
// @Summary 文件日志列表
// @Description 文件日志列表
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Param peer_id query int false "目标设备"
// @Param from_peer query int false "来源设备"
// @Success 200 {object} response.Response{data=model.AuditFileList}
// @Failure 500 {object} response.Response
// @Router /admin/audit_file/list [get]
// @Security token
func (a *Audit) FileList(c *gin.Context) {
	query := &admin.AuditQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.AuditService.AuditFileList(query.Page, query.PageSize, func(tx *gorm.DB) {
		admin.ApplyAuditCommon(tx, query)
		// 文件日志专有筛选
		if query.Path != "" {
			tx.Where("path like ?", "%"+query.Path+"%")
		}
		if query.IsFile != nil {
			tx.Where("is_file = ?", *query.IsFile)
		}
		admin.ApplyAuditOrder(tx, query)
	})
	// 版本 / 主机名 / 别名：老记录没存过，用 peers 表现值兜底
	service.AllService.AuditService.FillFileListPeerInfo(res.AuditFiles)
	response.Success(c, res)
}

// FileDelete 删除
// @Tags 文件日志
// @Summary 文件日志删除
// @Description 文件日志删除
// @Accept  json
// @Produce  json
// @Param body body model.AuditFile true "文件日志信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/audit_file/delete [post]
// @Security token
func (a *Audit) FileDelete(c *gin.Context) {
	f := &model.AuditFile{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id := f.Id
	errList := global.Validator.ValidVar(c, id, "required,gt=0")
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	l := service.AllService.AuditService.FileInfoById(f.Id)
	if l.Id > 0 {
		err := service.AllService.AuditService.DeleteAuditFile(l)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, err.Error())
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// BatchFileDelete 删除
// @Tags 文件日志
// @Summary 文件日志批量删除
// @Description 文件日志批量删除
// @Accept  json
// @Produce  json
// @Param body body admin.AuditFileLogIds true "文件日志"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/audit_file/batchDelete [post]
// @Security token
func (a *Audit) BatchFileDelete(c *gin.Context) {
	f := &admin.AuditFileLogIds{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(f.Ids) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	err := service.AllService.AuditService.BatchDeleteAuditFile(f.Ids)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
}
