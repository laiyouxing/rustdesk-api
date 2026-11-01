package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// Region 设备地域管理（后台批量下发）。
//
// 需求：地域不希望通过配置文件维护，而是**在网页上批量设置**：
// 选中一批设备 → 指定省份 → 一次写入。
//
// 优先级约定：客户端本地设置（设置页选的）> 这里下发的 > 未设置（纯延迟）。
// 因此这里写入的值是"服务端建议值"，客户端一旦自己选了会覆盖它。
type Region struct{}

// List 省份列表（后台下拉框数据源）
// @Tags 地域管理
// @Summary 省份列表
// @Router /admin/node/regions [get]
// @Security token
func (r *Region) List(c *gin.Context) {
	response.Success(c, gin.H{"regions": service.ChinaProvinces})
}

// BatchSet 批量为设备设置地域
// @Tags 地域管理
// @Summary 批量设置设备地域
// @Accept json
// @Produce json
// @Router /admin/peer/batch-region [post]
// @Security token
func (r *Region) BatchSet(c *gin.Context) {
	type Req struct {
		// Ids 设备 id 列表（远端 ID，不是行号），便于前端勾选后直接提交
		Ids    []string `json:"ids"`
		Region string   `json:"region"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(req.Ids) == 0 {
		response.Fail(c, 101, "请先选择设备")
		return
	}
	if !service.ValidProvinceCode(req.Region) {
		response.Fail(c, 101, "非法地域代码")
		return
	}
	res := service.DB.Model(&model.Peer{}).Where("id IN ?", req.Ids).Update("region", req.Region)
	if res.Error != nil {
		response.Fail(c, 101, "批量设置失败："+res.Error.Error())
		return
	}
	global.Logger.Infof("[Region] 批量设置 %d 台设备地域为 %q", res.RowsAffected, req.Region)
	response.Success(c, gin.H{"affected": res.RowsAffected})
}

// BatchSetByRowId 按行号批量设置（兼容前端勾选的 row_id）
// @Tags 地域管理
// @Summary 批量设置设备地域（按 row_id）
// @Router /admin/peer/batch-region-by-row [post]
// @Security token
func (r *Region) BatchSetByRowId(c *gin.Context) {
	type Req struct {
		RowIds []uint `json:"row_ids"`
		Region string `json:"region"`
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(req.RowIds) == 0 {
		response.Fail(c, 101, "请先选择设备")
		return
	}
	if !service.ValidProvinceCode(req.Region) {
		response.Fail(c, 101, "非法地域代码")
		return
	}
	res := service.DB.Model(&model.Peer{}).Where("row_id IN ?", req.RowIds).Update("region", req.Region)
	if res.Error != nil {
		response.Fail(c, 101, "批量设置失败："+res.Error.Error())
		return
	}
	global.Logger.Infof("[Region] 批量设置 %d 台设备地域为 %q（按行号）", res.RowsAffected, req.Region)
	response.Success(c, gin.H{"affected": res.RowsAffected})
}
