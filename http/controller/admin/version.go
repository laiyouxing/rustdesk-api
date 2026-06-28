package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Version struct {
}

// List 版本列表
// @Tags ADMIN 版本管理
// @Summary 版本列表
// @Description 版本列表
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/version/list [get]
// @Security token
func (v *Version) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	list, total := service.AllService.VersionService.List(uint(page), uint(pageSize))
	response.Success(c, gin.H{
		"list":  list,
		"total": total,
	})
}

// Create 创建版本
// @Tags ADMIN 版本管理
// @Summary 创建版本
// @Accept  json
// @Produce  json
// @Param body body model.Version true "版本信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/version/create [post]
// @Security token
func (v *Version) Create(c *gin.Context) {
	ver := &model.Version{}
	if err := c.ShouldBindJSON(ver); err != nil {
		response.Fail(c, "参数错误")
		return
	}
	if ver.Version == "" {
		response.Fail(c, "版本号不能为空")
		return
	}
	ver.Status = model.COMMON_STATUS_ENABLE
	service.AllService.VersionService.Create(ver)
	response.Success(c, nil)
}

// Update 更新版本
// @Tags ADMIN 版本管理
// @Summary 更新版本
// @Accept  json
// @Produce  json
// @Param body body model.Version true "版本信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/version/update [post]
// @Security token
func (v *Version) Update(c *gin.Context) {
	ver := &model.Version{}
	if err := c.ShouldBindJSON(ver); err != nil {
		response.Fail(c, "参数错误")
		return
	}
	if ver.Id == 0 {
		response.Fail(c, "ID不能为空")
		return
	}
	service.AllService.VersionService.Update(ver)
	response.Success(c, nil)
}

// Delete 删除版本
// @Tags ADMIN 版本管理
// @Summary 删除版本
// @Accept  json
// @Produce  json
// @Param id query int true "版本ID"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/version/delete [post]
// @Security token
func (v *Version) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.PostForm("id"))
	if id == 0 {
		response.Fail(c, "ID不能为空")
		return
	}
	service.AllService.VersionService.Delete(uint(id))
	response.Success(c, nil)
}

// SetEnable 启用/禁用版本
// @Tags ADMIN 版本管理
// @Summary 启用/禁用版本
// @Accept  json
// @Produce  json
// @Param id query int true "版本ID"
// @Param status query int true "1=启用 2=禁用"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/version/setEnable [post]
// @Security token
func (v *Version) SetEnable(c *gin.Context) {
	id, _ := strconv.Atoi(c.PostForm("id"))
	status, _ := strconv.Atoi(c.DefaultPostForm("status", "1"))
	if id == 0 {
		response.Fail(c, "ID不能为空")
		return
	}
	ver := service.AllService.VersionService.FindById(uint(id))
	if ver == nil || ver.Id == 0 {
		response.Fail(c, "版本不存在")
		return
	}
	ver.Status = status
	service.AllService.VersionService.Update(ver)
	response.Success(c, nil)
}
