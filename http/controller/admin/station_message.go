package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type StationMessage struct {
}

func (m *StationMessage) List(ctx *gin.Context) {
	page := 1
	pageSize := 20
	var total int64
	var list []model.StationMessage
	query := service.DB.Model(&model.StationMessage{})
	if ctx.Query("type") != "" {
		query = query.Where("type = ?", ctx.Query("type"))
	}
	query.Count(&total)
	query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list)
	response.Success(ctx, gin.H{"list": list, "total": total})
}

func (m *StationMessage) UnreadCount(ctx *gin.Context) {
	var count int64
	service.DB.Model(&model.StationMessage{}).Where("is_read = 0").Count(&count)
	response.Success(ctx, gin.H{"count": count})
}

func (m *StationMessage) MarkRead(ctx *gin.Context) {
	form := &struct {
		Id uint `json:"id"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil {
		// mark all as read
		service.DB.Model(&model.StationMessage{}).Where("is_read = 0").Update("is_read", 1)
	} else if form.Id > 0 {
		service.DB.Model(&model.StationMessage{}).Where("row_id = ?", form.Id).Update("is_read", 1)
	}
	response.Success(ctx, nil)
}
