package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Announcement struct {
}

// List 公告列表
func (a *Announcement) List(c *gin.Context) {
	announcements := service.AllService.AnnouncementService.List()
	response.Success(c, gin.H{
		"announcements": announcements,
	})
}

// Info 公告详情
func (a *Announcement) Info(c *gin.Context) {
	id, _ := strconv.Atoi(c.Query("id"))
	announcement := service.AllService.AnnouncementService.Info(id)
	if announcement.Id == 0 {
		response.Fail(c, 404, "公告不存在")
		return
	}
	response.Success(c, announcement)
}

// Create 创建公告
func (a *Announcement) Create(c *gin.Context) {
	announcement := &model.Announcement{}
	if err := c.ShouldBindJSON(announcement); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if announcement.Title == "" {
		response.Fail(c, 401, "标题不能为空")
		return
	}
	service.AllService.AnnouncementService.Create(announcement)
	response.Success(c, announcement)
}

// Update 更新公告
func (a *Announcement) Update(c *gin.Context) {
	announcement := &model.Announcement{}
	if err := c.ShouldBindJSON(announcement); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if announcement.Id == 0 {
		response.Fail(c, 401, "ID不能为空")
		return
	}
	service.AllService.AnnouncementService.Update(announcement)
	response.Success(c, nil)
}

// Delete 删除公告
func (a *Announcement) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Query("id"))
	announcement := service.AllService.AnnouncementService.Info(id)
	if announcement.Id == 0 {
		response.Fail(c, 404, "公告不存在")
		return
	}
	service.AllService.AnnouncementService.Delete(announcement)
	response.Success(c, nil)
}
