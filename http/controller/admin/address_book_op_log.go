package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
)

// AddressBookOpLog 地址簿操作审计日志
type AddressBookOpLog struct {
}

// List 地址簿操作日志列表
// @Tags 地址簿操作日志
// @Summary 地址簿操作日志列表
// @Description 地址簿操作日志列表（新建/编辑/删除/批量/标签/分组/规则/分享）
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Param keyword query string false "关键字（操作者/用户名）"
// @Param action query string false "操作类型"
// @Success 200 {object} response.Response
// @Router /admin/address_book_op_log/list [get]
// @Security token
func (ct *AddressBookOpLog) List(c *gin.Context) {
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

	res := service.AllService.AddressBookOpLogService.List(uint(page), uint(pageSize), func(tx *gorm.DB) {
		if action != "" {
			tx.Where("action = ?", action)
		}
		if keyword != "" {
			tx.Where("operator_name LIKE ? OR username LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
		}
	})
	response.Success(c, res)
}
