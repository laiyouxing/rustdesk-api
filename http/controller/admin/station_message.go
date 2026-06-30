package admin

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type StationMessage struct {
}

func (m *StationMessage) List(ctx *gin.Context) {
	page, _ := parseIntQuery(ctx, "page", 1)
	pageSize, _ := parseIntQuery(ctx, "page_size", 20)
	var total int64
	var list []model.StationMessage
	query := service.DB.Model(&model.StationMessage{})

	user := ctx.MustGet("curUser").(*model.User)
	isAdmin := service.AllService.UserService.IsAdmin(user)

	// Non-admin users only see messages addressed to them or broadcasts
	if !isAdmin {
		query = query.Where("(receiver_id = ? OR receiver_id = 0)", user.Id)
	}

	// Admin users can filter by type
	if ctx.Query("type") != "" {
		query = query.Where("type = ?", ctx.Query("type"))
	}
	if ctx.Query("sender") != "" {
		query = query.Where("sender_name like ?", "%"+ctx.Query("sender")+"%")
	}
	if ctx.Query("is_read") != "" {
		query = query.Where("is_read = ?", ctx.Query("is_read"))
	}

	query.Count(&total)
	query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list)
	response.Success(ctx, gin.H{"list": list, "total": total})
}

func parseIntQuery(ctx *gin.Context, key string, defaultVal int) (int, error) {
	val := ctx.Query(key)
	if val == "" {
		return defaultVal, nil
	}
	n := 0
	_, err := fmt.Sscanf(val, "%d", &n)
	if err != nil || n <= 0 {
		return defaultVal, err
	}
	return n, nil
}

func (m *StationMessage) UnreadCount(ctx *gin.Context) {
	var count int64
	user := ctx.MustGet("curUser").(*model.User)
	service.DB.Model(&model.StationMessage{}).
		Where("(receiver_id = ? OR receiver_id = 0) AND is_read = 0", user.Id).
		Count(&count)
	response.Success(ctx, gin.H{"count": count})
}

func (m *StationMessage) MarkRead(ctx *gin.Context) {
	user := ctx.MustGet("curUser").(*model.User)
	form := &struct {
		Id  uint `json:"id"`
		All bool `json:"all"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if form.All {
		// 全部标记已读（需前端显式传入 all=true）
		service.DB.Model(&model.StationMessage{}).
			Where("(receiver_id = ? OR receiver_id = 0) AND is_read = 0", user.Id).
			Update("is_read", 1)
	} else if form.Id > 0 {
		service.DB.Model(&model.StationMessage{}).
			Where("row_id = ? AND (receiver_id = ? OR receiver_id = 0)", form.Id, user.Id).
			Update("is_read", 1)
	}
	response.Success(ctx, nil)
}

// Send sends a station message from one user to another
func (m *StationMessage) Send(ctx *gin.Context) {
	sender := ctx.MustGet("curUser").(*model.User)
	form := &struct {
		ReceiverId uint   `json:"receiver_id"`
		Title      string `json:"title"`
		Content    string `json:"content"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if form.ReceiverId == 0 {
		response.Fail(ctx, 101, "请选择接收人")
		return
	}
	if form.Title == "" && form.Content == "" {
		response.Fail(ctx, 101, "请输入消息内容")
		return
	}
	msg := &model.StationMessage{
		Type:       "user",
		Title:      form.Title,
		Content:    form.Content,
		SenderId:   sender.Id,
		SenderName: sender.Username,
		ReceiverId: form.ReceiverId,
	}
	service.DB.Create(msg)
	response.Success(ctx, nil)
}

// Broadcast sends a station message to all users (admin only)
func (m *StationMessage) Broadcast(ctx *gin.Context) {
	sender := ctx.MustGet("curUser").(*model.User)
	if !service.AllService.UserService.IsAdmin(sender) {
		response.Fail(ctx, 101, "无权限")
		return
	}
	form := &struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if form.Title == "" && form.Content == "" {
		response.Fail(ctx, 101, "请输入消息内容")
		return
	}
	msg := &model.StationMessage{
		Type:       "broadcast",
		Title:      form.Title,
		Content:    fmt.Sprintf("【全体推送】%s\n%s", form.Title, form.Content),
		SenderId:   sender.Id,
		SenderName: sender.Username + "(管理员)",
		ReceiverId: 0, // 0 = broadcast to all
	}
	service.DB.Create(msg)
	response.Success(ctx, nil)
}

// Cleanup deletes messages older than the specified years (admin only)
func (m *StationMessage) Cleanup(ctx *gin.Context) {
	sender := ctx.MustGet("curUser").(*model.User)
	if !service.AllService.UserService.IsAdmin(sender) {
		response.Fail(ctx, 101, "无权限")
		return
	}
	form := &struct {
		Years int `json:"years"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil || (form.Years != 1 && form.Years != 3) {
		response.Fail(ctx, 101, "参数错误，仅支持清理超过1年或3年的消息")
		return
	}
	cutoff := time.Now().AddDate(-form.Years, 0, 0).Unix()
	result := service.DB.Where("created_at < ? AND created_at > 0", cutoff).Delete(&model.StationMessage{})
	if result.Error != nil {
		response.Fail(ctx, 101, "清理失败")
		return
	}
	response.Success(ctx, gin.H{"deleted": result.RowsAffected})
}
