package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
)

// AccountOpLog 账户操作审计日志
type AccountOpLog struct {
}

// List 账户操作日志列表
// @Tags 账户操作日志
// @Summary 账户操作日志列表
// @Description 账户操作日志列表（新建/延长/终止会员）
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Param keyword query string false "关键字（操作者/目标用户名）"
// @Param action query string false "操作类型 create/extend/terminate"
// @Success 200 {object} response.Response
// @Router /admin/account_op_log/list [get]
// @Security token
func (ct *AccountOpLog) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	keyword := c.Query("keyword")
	action := c.Query("action")

	res := service.AllService.AccountOpLogService.List(uint(page), uint(pageSize), func(tx *gorm.DB) {
		if action != "" {
			tx.Where("action = ?", action)
		}
		if keyword != "" {
			tx.Where("operator_name LIKE ? OR target_username LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
		}
	})
	response.Success(c, res)
}
