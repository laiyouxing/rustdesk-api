package admin

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	respApi "github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// AdminInviteCodeController 后台邀请码管理
type AdminInviteCodeController struct {
}

// NewAdminInviteCodeController 创建控制器
func NewAdminInviteCodeController() *AdminInviteCodeController {
	return &AdminInviteCodeController{}
}

// List 分页查询邀请码列表
func (ac *AdminInviteCodeController) List(c *gin.Context) {
	status := c.Query("status")
	plan := c.Query("plan")
	usedByStr := c.Query("used_by")
	pageStr := c.Query("page")
	pageSizeStr := c.Query("size")

	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 {
		pageSize = 20
	}
	var usedBy uint
	if usedByStr != "" {
		uid, err := strconv.ParseUint(usedByStr, 10, 64)
		if err == nil {
			usedBy = uint(uid)
		}
	}

	filter := service.InviteCodeFilter{
		Status:   status,
		Plan:     plan,
		UsedBy:   usedBy,
		Page:     page,
		PageSize: pageSize,
	}

	ics := &service.InviteCodeService{}
	list, total, err := ics.List(filter)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	items := make([]respApi.CodeListItem, 0, len(list))
	for _, ic := range list {
		items = append(items, respApi.CodeListItem{
			ID:           ic.Id,
			Code:         ic.Code,
			Plan:         ic.Plan,
			Status:       ic.Status,
			UsedBy:       ic.UsedBy,
			ExpireAt:     ic.ExpireAt,
			BoundOrderID: ic.BoundOrderID,
			CreatedAt:    ic.CreatedAt,
		})
	}

	response.Success(c, response.PageData{
		Page:  page,
		Total: int(total),
		List:  items,
	})
}

// Create 手动生成邀请码
func (ac *AdminInviteCodeController) Create(c *gin.Context) {
	req := &api.AdminCreateCodeReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 400, response.TranslateMsg(c, "ParamError"))
		return
	}

	if req.Plan == "" {
		req.Plan = "pro"
	}
	if req.ExpireDays <= 0 {
		req.ExpireDays = 30
	}

	ics := &service.InviteCodeService{}
	ic, err := ics.Generate(req.Plan, 0, "", req.ExpireDays)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	response.Success(c, gin.H{
		"code":      ic.Code,
		"plan":      ic.Plan,
		"expire_at": ic.ExpireAt,
	})
}

// Revoke 失效邀请码
func (ac *AdminInviteCodeController) Revoke(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, 400, "invalid id")
		return
	}

	ics := &service.InviteCodeService{}
	if err := ics.Revoke(uint(id)); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, nil)
}

// Export 导出邀请码 CSV（对账用）
func (ac *AdminInviteCodeController) Export(c *gin.Context) {
	status := c.Query("status")
	plan := c.Query("plan")

	filter := service.InviteCodeFilter{
		Status:   status,
		Plan:     plan,
		Page:     1,
		PageSize: 100000, // 一次性导出
	}

	ics := &service.InviteCodeService{}
	list, _, err := ics.List(filter)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=invite_codes_%s.csv", time.Now().Format("20060102150405")))

	// 写入 BOM 使 Excel 正确识别 UTF-8
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(c.Writer)
	writer.Write([]string{"ID", "Code", "Plan", "Status", "UsedBy", "BoundOrderID", "ExpireAt", "CreatedAt"})

	for _, ic := range list {
		writer.Write([]string{
			strconv.Itoa(int(ic.Id)),
			ic.Code,
			ic.Plan,
			ic.Status,
			strconv.Itoa(int(ic.UsedBy)),
			ic.BoundOrderID,
			ic.ExpireAt.Format("2006-01-02 15:04:05"),
			ic.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	writer.Flush()
}
