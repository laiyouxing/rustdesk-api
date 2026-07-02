package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
)

type ClientDownload struct{}

// Page 下载链接管理页面
func (v *ClientDownload) Page(c *gin.Context) {
	c.HTML(http.StatusOK, "client_download_admin.html", gin.H{})
}

// List 获取列表
func (v *ClientDownload) List(c *gin.Context) {
	page := 1
	pageSize := 10
	queryPage := c.Query("page")
	if queryPage != "" {
		if p, err := parseInt(queryPage); err == nil {
			page = p
		}
	}
	querySize := c.Query("page_size")
	if querySize != "" {
		if ps, err := parseInt(querySize); err == nil {
			pageSize = ps
		}
	}
	list, total := service.AllService.ClientDownloadService.List(uint(page), uint(pageSize))
	response.Success(c, gin.H{
		"list":  list,
		"total": total,
	})
}

// Create 创建
func (v *ClientDownload) Create(c *gin.Context) {
	item := &model.ClientDownload{}
	if err := c.ShouldBindJSON(item); err != nil {
		response.Fail(c, 101, "参数错误")
		return
	}
	if item.Name == "" || item.Url == "" {
		response.Fail(c, 101, "名称和下载地址不能为空")
		return
	}
	item.Status = int(model.COMMON_STATUS_ENABLE)
	service.AllService.ClientDownloadService.Create(item)
	response.Success(c, nil)
}

// Update 更新
func (v *ClientDownload) Update(c *gin.Context) {
	item := &model.ClientDownload{}
	if err := c.ShouldBindJSON(item); err != nil {
		response.Fail(c, 101, "参数错误")
		return
	}
	if item.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	service.AllService.ClientDownloadService.Update(item)
	response.Success(c, nil)
}

// Delete 删除
func (v *ClientDownload) Delete(c *gin.Context) {
	form := &struct {
		Id uint `json:"id"`
	}{}
	if err := c.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	service.AllService.ClientDownloadService.Delete(form.Id)
	response.Success(c, nil)
}

// SetEnable 启用/禁用
func (v *ClientDownload) SetEnable(c *gin.Context) {
	form := &struct {
		Id     uint `json:"id"`
		Status int  `json:"status"`
	}{Status: 1}
	if err := c.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	item := service.AllService.ClientDownloadService.FindById(form.Id)
	if item == nil || item.Id == 0 {
		response.Fail(c, 101, "记录不存在")
		return
	}
	item.Status = form.Status
	service.AllService.ClientDownloadService.Update(item)
	response.Success(c, nil)
}
