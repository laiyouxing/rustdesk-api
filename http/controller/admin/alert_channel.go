package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type AlertChannel struct{}

func (ct *AlertChannel) List(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	page, pageSize := parsePageParams(c)
	var total int64
	var list []model.AlertChannel
	query := service.DB.Model(&model.AlertChannel{}).Where("user_id = ?", u.Id)
	query.Count(&total)
	query.Order("id desc").Scopes(service.Paginate(page, pageSize)).Find(&list)
	response.Success(c, gin.H{"list": list, "total": total})
}

func (ct *AlertChannel) Create(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	f := &model.AlertChannel{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, "参数错误")
		return
	}
	f.UserId = u.Id
	if f.Name == "" {
		response.Fail(c, 101, "请输入通道名称")
		return
	}
	service.DB.Create(f)
	response.Success(c, f)
}

func (ct *AlertChannel) Update(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	f := &model.AlertChannel{}
	if err := c.ShouldBindJSON(f); err != nil || f.RowId == 0 {
		response.Fail(c, 101, "参数错误")
		return
	}
	old := &model.AlertChannel{}
	service.DB.Where("row_id = ? AND user_id = ?", f.RowId, u.Id).First(old)
	if old.RowId == 0 {
		response.Fail(c, 101, "记录不存在")
		return
	}
	if f.SmtpPass == "" {
		f.SmtpPass = old.SmtpPass // 密码留空则不修改
	}
	service.DB.Model(old).Updates(f)
	response.Success(c, nil)
}

func (ct *AlertChannel) Delete(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	f := &struct {
		Id uint `json:"id"`
	}{}
	if err := c.ShouldBindJSON(f); err != nil || f.Id == 0 {
		response.Fail(c, 101, "参数错误")
		return
	}
	// 检查是否有告警规则在使用此通道
	var usage int64
	service.DB.Model(&model.AlertConfig{}).Where("channel_id = ? AND user_id = ?", f.Id, u.Id).Count(&usage)
	if usage > 0 {
		response.Fail(c, 101, "该通道正在被告警规则使用，无法删除")
		return
	}
	ch := &model.AlertChannel{}
	service.DB.Where("row_id = ? AND user_id = ?", f.Id, u.Id).Delete(ch)
	response.Success(c, nil)
}

// AllList 返回用户所有通道（不分页），供选择器使用
func (ct *AlertChannel) AllList(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	var list []model.AlertChannel
	service.DB.Where("user_id = ?", u.Id).Order("id desc").Find(&list)
	response.Success(c, gin.H{"list": list})
}

func parsePageParams(c *gin.Context) (page, pageSize uint) {
	page = 1
	pageSize = 20
	// default values, can be extended
	return
}
