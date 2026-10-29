package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
)

// PunchDirectOk 直连上报记录（合规审计）
//
// 展示客户端在直连建立成功后上报的两端网络标识。这张表既是合规留痕，
// 也是服务端判定"未登记直连"并强制断开的执法依据。
//
// 安全约束：表中的 mac_hashes 是客户端计算的哈希，**不含原始 MAC**；
// 表内只有元数据，没有会话内容（连接端到端加密，服务端不解）。
type PunchDirectOk struct {
}

// List 直连上报记录列表
// @Tags 直连上报
// @Summary 直连上报记录列表
// @Description 直连上报记录列表
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Param peer_id query string false "上报设备"
// @Param target query string false "对端设备"
// @Param conn_type query string false "direct_v4/direct_v6/relay"
// @Success 200 {object} response.Response{data=model.PunchDirectOkList}
// @Failure 500 {object} response.Response
// @Router /admin/punch_direct_ok/list [get]
// @Security token
func (a *PunchDirectOk) List(c *gin.Context) {
	query := &admin.PunchDirectOkQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.PunchExchangeService.DirectOkList(query.Page, query.PageSize, func(tx *gorm.DB) {
		if query.PeerId != "" {
			tx.Where("peer_id like ?", "%"+query.PeerId+"%")
		}
		if query.Target != "" {
			tx.Where("target like ?", "%"+query.Target+"%")
		}
		if query.ConnType != "" {
			tx.Where("conn_type = ?", query.ConnType)
		}
		tx.Order("id desc")
	})
	response.Success(c, res)
}

// Delete 删除单条直连上报记录
// @Tags 直连上报
// @Summary 删除直连上报记录
// @Router /admin/punch_direct_ok/delete [post]
// @Security token
func (a *PunchDirectOk) Delete(c *gin.Context) {
	f := &model.PunchDirectOk{}
	if err := c.ShouldBindJSON(f); err != nil || f.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if err := service.DB.Delete(&model.PunchDirectOk{}, f.Id).Error; err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// BatchDelete 批量删除直连上报记录
// @Tags 直连上报
// @Summary 批量删除直连上报记录
// @Router /admin/punch_direct_ok/batchDelete [post]
// @Security token
func (a *PunchDirectOk) BatchDelete(c *gin.Context) {
	f := &admin.PunchDirectOkLogIds{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(f.Ids) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if err := service.DB.Delete(&model.PunchDirectOk{}, f.Ids).Error; err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}
