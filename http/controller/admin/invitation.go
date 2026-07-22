package admin

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Invitation struct {
}

func (ct *Invitation) List(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("page_size", "20")
	p, _ := strconv.Atoi(page)
	ps, _ := strconv.Atoi(pageSize)
	invSvc := service.NewInvitationService()
	pageInt := int64(p)
	pageSizeInt := int64(ps)
	list, total := invSvc.List(pageInt, pageSizeInt)
	response.Success(c, &model.InvitationList{
		Invitations: list,
		Pagination: model.Pagination{
			Page:     pageInt,
			PageSize: pageSizeInt,
			Total:    total,
		},
	})
}

func (ct *Invitation) Create(c *gin.Context) {
	f := &admin.InvitationForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	inv := &model.Invitation{
		MaxUsers:      1,
		ExpiredAt:     f.ExpiredAt,
		UserExpiredAt: f.UserExpiredAt,
		Remark:        f.Remark,
	}
	if f.ExpiredAt == 0 {
		inv.ExpiredAt = time.Now().Unix() + 86400
	}
	if f.Code != "" {
		inv.Code = f.Code
	}
	invSvc := service.NewInvitationService()
	if err := invSvc.Create(inv); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, inv)
}

func (ct *Invitation) BatchCreate(c *gin.Context) {
	f := &admin.InvitationBatchForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	invSvc := service.NewInvitationService()
	list, err := invSvc.BatchCreate(f.Count, f.ExpiredAt, f.UserExpiredAt, f.Remark)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, gin.H{
		"list":  list,
		"count": len(list),
	})
}

func (ct *Invitation) Delete(c *gin.Context) {
	f := &admin.InvitationDeleteForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	invSvc := service.NewInvitationService()
	inv := invSvc.InfoById(f.Id)
	if inv.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if err := invSvc.Delete(f.Id); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

func (ct *Invitation) Info(c *gin.Context) {
	idStr := c.Query("id")
	id, _ := strconv.Atoi(idStr)
	if id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	invSvc := service.NewInvitationService()
	inv := invSvc.InfoById(uint(id))
	if inv.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	response.Success(c, inv)
}
