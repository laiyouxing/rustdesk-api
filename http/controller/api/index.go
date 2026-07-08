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

	// 策略下发：按用户指定优先级查找
	// 绑定优先级: user(最高) > group > tag > global(兜底)
	// 同类型的多个策略按数值优先级取最高
	var foundStrategy *model.Strategy

	// 查询所有启用的策略，按数值优先级降序
	var allEnabled []model.Strategy
	service.DB.Where("status = 1").Order("priority desc").Find(&allEnabled)

	// 收集该设备关联的标签ID（通过地址簿，存的是标签名）
	var peerTagIds []uint
	var abs []model.AddressBook
	service.DB.Where("id = ?", peer.Id).Find(&abs)
	for _, ab := range abs {
		for _, tagName := range ab.Tags {
			var tag model.Tag
			service.DB.Where("name = ?", tagName).Limit(1).Find(&tag)
			if tag.Id > 0 {
				peerTagIds = append(peerTagIds, tag.Id)
			}
		}
	}

	// 按绑定类型优先级查找：user → group → tag → global
	// 每种类型取数值优先级最高的第一个
	bindOrder := []string{"user", "group", "tag", "global"}
	for _, bindType := range bindOrder {
		if foundStrategy != nil {
			break
		}
		for _, s := range allEnabled {
			if s.BindType != bindType {
				continue
			}
			match := false
			switch bindType {
			case "user":
				// 用户绑定：设备所属用户匹配（不含别人分享的策略）
				if peer.UserId > 0 && s.BindId == peer.UserId {
					match = true
				}
			case "group":
				// 设备分组绑定
				if peer.GroupId > 0 && s.BindId == peer.GroupId {
					match = true
				}
			case "tag":
				// 标签绑定：设备的地址簿中有该标签
				for _, tid := range peerTagIds {
					if tid == s.BindId {
						match = true
						break
					}
				}
			case "global":
				// 全局绑定：适用于所有设备
				match = true
			}
			if match {
				foundStrategy = &s
				break
			}
		}
	}

	if foundStrategy != nil {
		// 将 ConfigItems (key=value 每行) 解析为 map
		configMap := make(map[string]string)
		for _, line := range strings.Split(foundStrategy.ConfigItems, "\n") {
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
