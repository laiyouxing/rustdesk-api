package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	requstform "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
)

type Peer struct {
}

// SysInfo
// @Tags System
// @Summary 提交系统信息
// @Description 提交系统信息
// @Accept  json
// @Produce  json
// @Param body body api.PeerForm true "系统信息表单"
// @Success 200 {string} string "SYSINFO_UPDATED,ID_NOT_FOUND"
// @Failure 500 {object} response.ErrorResponse
// @Router /sysinfo [post]
func (p *Peer) SysInfo(c *gin.Context) {
	f := &requstform.PeerForm{}
	err := c.ShouldBindBodyWith(f, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	fpe := f.ToPeer()
	// 新装设备在拿到 ID 之前会以空 id 上报。此时**不能**按 id 建档——否则
	// Peer 表会堆积 id='' 的垃圾行，且 FindById("") 会命中它们。
	// 空 id 时只按 uuid 更新已有记录的元信息；ID 由登录后的
	// /api/device/claim-id 分配。
	if f.Id == "" {
		if fpe.Uuid != "" {
			if exist := service.AllService.PeerService.FindByUuid(fpe.Uuid); exist.RowId != 0 {
				upd := map[string]interface{}{}
				if fpe.Hostname != "" && fpe.Hostname != exist.Hostname {
					upd["hostname"] = fpe.Hostname
				}
				if fpe.Version != "" && fpe.Version != exist.Version {
					upd["version"] = fpe.Version
				}
				if len(upd) > 0 {
					service.DB.Model(&model.Peer{}).Where("row_id = ?", exist.RowId).Updates(upd)
				}
			}
		}
		c.String(http.StatusOK, "SYSINFO_UPDATED")
		return
	}
	pe := service.AllService.PeerService.FindById(f.Id)
	if pe.RowId == 0 {
		pe = f.ToPeer()
		pe.UserId = service.AllService.UserService.FindLatestUserIdFromLoginLogByUuid(pe.Uuid, pe.Id)
		err = service.AllService.PeerService.Create(pe)
		if err != nil {
			response.Error(c, response.TranslateMsg(c, "OperationFailed")+err.Error())
			return
		}
	} else {
		if pe.UserId == 0 {
			pe.UserId = service.AllService.UserService.FindLatestUserIdFromLoginLogByUuid(pe.Uuid, pe.Id)
		}
		fpe.RowId = pe.RowId
		fpe.UserId = pe.UserId
		err = service.AllService.PeerService.Update(fpe)
		if err != nil {
			response.Error(c, response.TranslateMsg(c, "OperationFailed")+err.Error())
			return
		}
	}
	//SYSINFO_UPDATED 上传成功
	//ID_NOT_FOUND 下次心跳会上传
	//直接响应文本
	c.String(http.StatusOK, "SYSINFO_UPDATED")
}

// SysInfoVer
// @Tags System
// @Summary 获取系统版本信息
// @Description 获取系统版本信息
// @Accept  json
// @Produce  json
// @Success 200 {string} string ""
// @Failure 500 {object} response.ErrorResponse
// @Router /sysinfo_ver [post]
func (p *Peer) SysInfoVer(c *gin.Context) {
	//读取resources/version文件
	v := service.AllService.AppService.GetAppVersion()
	// 加上启动时间，方便client上传信息
	v = fmt.Sprintf("%s\n%s", v, service.AllService.AppService.GetStartTime())
	c.String(http.StatusOK, v)
}
