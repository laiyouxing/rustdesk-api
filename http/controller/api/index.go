package api

import (
	"github.com/gin-gonic/gin"
	requstform "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
	"strings"
	"time"
)

type Index struct {
}

// Index 首页
// @Tags 首页
// @Summary 首页
// @Description 首页
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router / [get]
func (i *Index) Index(c *gin.Context) {
	response.Success(
		c,
		"Hello Gwen",
	)
}

// Heartbeat 心跳
// @Tags 首页
// @Summary 心跳
// @Description 心跳
// @Accept  json
// @Produce  json
// @Success 200 {object} nil
// @Failure 500 {object} response.Response
// @Router /heartbeat [post]
func (i *Index) Heartbeat(c *gin.Context) {
	info := &requstform.PeerInfoInHeartbeat{}
	err := c.ShouldBindJSON(info)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if info.Uuid == "" {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	peer := service.AllService.PeerService.FindById(info.Id)
	if peer == nil || peer.RowId == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	//如果在40s以内则不更新
	if time.Now().Unix()-peer.LastOnlineTime >= 30 {
		upp := &model.Peer{RowId: peer.RowId, LastOnlineTime: time.Now().Unix(), LastOnlineIp: c.ClientIP()}
		service.AllService.PeerService.Update(upp)
	}

	resp := gin.H{}

	// 查找用户和分组，获取关联的策略
	user := service.AllService.UserService.InfoById(peer.UserId)
	if user != nil && user.Id > 0 {
		// 查找策略：先查用户直接绑定的，再查分组绑定的
		var strategies []model.Strategy
		tx := service.DB.Where("status = 1")
		if peer.GroupId > 0 {
			// 优先查找分组关联的策略（按优先级降序）
			tx = tx.Where("user_id = ?", peer.GroupId)
		}
		tx.Order("priority desc").Find(&strategies)
		if len(strategies) == 0 && user.Id > 0 {
			// 没有分组策略则查用户自己的策略
			service.DB.Where("status = 1 AND user_id = ?", user.Id).Order("priority desc").Find(&strategies)
		}
		if len(strategies) > 0 {
			s := strategies[0] // 取优先级最高的
			// 将 ConfigItems (key=value 每行) 解析为 map
			configMap := make(map[string]string)
			for _, line := range strings.Split(s.ConfigItems, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					if key != "" {
						configMap[key] = val
					}
				}
			}
			if len(configMap) > 0 {
				resp["strategy"] = model.StrategyOptions{ConfigOptions: configMap}
				resp["modified_at"] = time.Now().Unix()
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// Version 版本
// @Tags 首页
// @Summary 版本
// @Description 版本
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /version [get]
func (i *Index) Version(c *gin.Context) {
	//读取resources/version文件
	v := service.AllService.AppService.GetAppVersion()
	response.Success(
		c,
		v,
	)
}

// VersionAdmin 版本管理页面
func (i *Index) VersionAdmin(c *gin.Context) {
	c.HTML(http.StatusOK, "version_admin.html", gin.H{})
}
