package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type AlertConfig struct{}

func (c *AlertConfig) List(ctx *gin.Context) {
	var configs []model.AlertConfig
	service.DB.Find(&configs)
	response.Success(ctx, gin.H{"list": configs})
}

func (c *AlertConfig) Create(ctx *gin.Context) {
	f := &model.AlertConfig{}
	if err := ctx.ShouldBindJSON(f); err != nil {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if f.ChannelId == 0 {
		response.Fail(ctx, 101, "请选择通知通道")
		return
	}
	f.UserId = 0 // 0 表示管理员共享
	// 从通道获取 channel 类型
	ch := &model.AlertChannel{}
	service.DB.Where("row_id = ?", f.ChannelId).First(ch)
	if ch.RowId == 0 {
		response.Fail(ctx, 101, "通知通道不存在")
		return
	}
	f.Channel = ch.Channel
	f.Name = ch.Name
	if err := service.DB.Create(f).Error; err != nil {
		global.Logger.Error("AlertConfig Create failed: ", err)
		response.Fail(ctx, 500, "保存失败："+err.Error())
		return
	}
	response.Success(ctx, f)
}

func (c *AlertConfig) Update(ctx *gin.Context) {
	f := &model.AlertConfig{}
	if err := ctx.ShouldBindJSON(f); err != nil || f.RowId == 0 {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	// 如果更新了 channel_id，同步更新 channel 名称
	if f.ChannelId > 0 {
		ch := &model.AlertChannel{}
		service.DB.Where("row_id = ?", f.ChannelId).First(ch)
		if ch.RowId > 0 {
			f.Channel = ch.Channel
			f.Name = ch.Name
		}
	}
	if err := service.DB.Model(&model.AlertConfig{}).Where("row_id = ?", f.RowId).Updates(f).Error; err != nil {
		global.Logger.Error("AlertConfig Update failed: ", err)
		response.Fail(ctx, 500, "保存失败："+err.Error())
		return
	}
	response.Success(ctx, nil)
}

func (c *AlertConfig) Delete(ctx *gin.Context) {
	form := &struct {
		Id uint `json:"id"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(ctx, 101, "ID不能为空")
		return
	}
	// 先确认记录存在
	var cfg model.AlertConfig
	service.DB.Where("row_id = ?", form.Id).First(&cfg)
	if cfg.RowId == 0 {
		response.Fail(ctx, 101, "记录不存在")
		return
	}
	// 级联删除监控目标，避免留下孤儿数据
	service.DB.Where("alert_id = ?", cfg.RowId).Delete(&model.AlertTarget{})
	service.DB.Delete(&cfg)
	response.Success(ctx, nil)
}
