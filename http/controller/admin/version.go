package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Version struct {
}

func (v *Version) List(c *gin.Context) {
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
	list, total := service.AllService.AppReleaseService.List(uint(page), uint(pageSize))
	response.Success(c, gin.H{
		"list":  list,
		"total": total,
	})
}

func (v *Version) Create(c *gin.Context) {
	ver := &model.AppRelease{}
	if err := c.ShouldBindJSON(ver); err != nil {
		response.Fail(c, 101, "参数错误")
		return
	}
	if ver.Version == "" {
		response.Fail(c, 101, "版本号不能为空")
		return
	}
	ver.Status = int(model.COMMON_STATUS_ENABLE)
	service.AllService.AppReleaseService.Create(ver)
	response.Success(c, nil)
}

func (v *Version) Update(c *gin.Context) {
	ver := &model.AppRelease{}
	if err := c.ShouldBindJSON(ver); err != nil {
		response.Fail(c, 101, "参数错误")
		return
	}
	if ver.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	service.AllService.AppReleaseService.Update(ver)
	response.Success(c, nil)
}

func (v *Version) Delete(c *gin.Context) {
	form := &struct {
		Id uint `json:"id"`
	}{}
	if err := c.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	service.AllService.AppReleaseService.Delete(form.Id)
	response.Success(c, nil)
}

func (v *Version) SetEnable(c *gin.Context) {
	form := &struct {
		Id     uint `json:"id"`
		Status int  `json:"status"`
	}{Status: 1}
	if err := c.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(c, 101, "ID不能为空")
		return
	}
	ver := service.AllService.AppReleaseService.FindById(form.Id)
	if ver == nil || ver.Id == 0 {
		response.Fail(c, 101, "版本不存在")
		return
	}
	ver.Status = form.Status
	service.AllService.AppReleaseService.Update(ver)
	response.Success(c, nil)
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
