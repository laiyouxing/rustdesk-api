package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"strconv"
)

type Strategy struct {
}

func (ct *Strategy) List(c *gin.Context) {
	page := c.Query("page")
	pageSize := c.Query("page_size")
	pid, _ := strconv.Atoi(page)
	ps, _ := strconv.Atoi(pageSize)
	if pid <= 0 {
		pid = 1
	}
	if ps <= 0 {
		ps = 10
	}
	res := service.AllService.StrategyService.List(uint(pid), uint(ps), nil)
	response.Success(c, res)
}

func (ct *Strategy) Create(c *gin.Context) {
	f := &model.Strategy{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, "参数错误"+err.Error())
		return
	}
	if f.Name == "" {
		response.Fail(c, 101, "策略名称不能为空")
		return
	}
	if err := service.AllService.StrategyService.Create(f); err != nil {
		response.Fail(c, 101, "创建失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

func (ct *Strategy) Update(c *gin.Context) {
	f := &model.Strategy{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, "参数错误"+err.Error())
		return
	}
	if f.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	if err := service.AllService.StrategyService.Update(f); err != nil {
		response.Fail(c, 101, "更新失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

func (ct *Strategy) Delete(c *gin.Context) {
	form := &struct {
		Id uint `json:"id"`
	}{}
	if err := c.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	s := service.AllService.StrategyService.InfoById(form.Id)
	if s.Id == 0 {
		response.Fail(c, 101, "策略不存在")
		return
	}
	if err := service.AllService.StrategyService.Delete(s); err != nil {
		response.Fail(c, 101, "删除失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}
