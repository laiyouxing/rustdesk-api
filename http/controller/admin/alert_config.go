package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type AlertConfig struct {
}

func (c *AlertConfig) List(ctx *gin.Context) {
	var configs []model.AlertConfig
	if user, ok := ctx.Get("curUser"); ok {
		if u, ok := user.(*model.User); ok {
			service.DB.Where("user_id = ?", u.Id).Find(&configs)
			response.Success(ctx, gin.H{"list": configs})
			return
		}
	}
	service.DB.Find(&configs)
	response.Success(ctx, gin.H{"list": configs})
}

func (c *AlertConfig) Create(ctx *gin.Context) {
	f := &model.AlertConfig{}
	if err := ctx.ShouldBindJSON(f); err != nil {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if f.Channel == "" {
		response.Fail(ctx, 101, "请选择通道类型")
		return
	}
	// Save the creator's user ID for scoping
	if user, ok := ctx.Get("curUser"); ok {
		if u, ok := user.(*model.User); ok {
			f.UserId = u.Id
		}
	}
	service.DB.Create(f)
	response.Success(ctx, nil)
}

func (c *AlertConfig) Update(ctx *gin.Context) {
	f := &model.AlertConfig{}
	if err := ctx.ShouldBindJSON(f); err != nil {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if f.RowId == 0 {
		response.Fail(ctx, 101, "ID不能为空")
		return
	}
	// Only allow updating own configs
	if user, ok := ctx.Get("curUser"); ok {
		if u, ok := user.(*model.User); ok {
			service.DB.Model(&model.AlertConfig{}).Where("row_id = ? AND user_id = ?", f.RowId, u.Id).Updates(f)
			response.Success(ctx, nil)
			return
		}
	}
	response.Fail(ctx, 101, "无权限")
}

func (c *AlertConfig) Delete(ctx *gin.Context) {
	form := &struct {
		Id uint `json:"id"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(ctx, 101, "ID不能为空")
		return
	}
	if user, ok := ctx.Get("curUser"); ok {
		if u, ok := user.(*model.User); ok {
			service.DB.Where("row_id = ? AND user_id = ?", form.Id, u.Id).Delete(&model.AlertConfig{})
			response.Success(ctx, nil)
			return
		}
	}
	response.Fail(ctx, 101, "无权限")
}
