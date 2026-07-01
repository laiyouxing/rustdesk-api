package my

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
	"time"
)

type Peer struct {
}

// List 列表
// @Tags 我的设备
// @Summary 设备列表
// @Description 设备列表
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Param time_ago query int false "时间"
// @Param id query string false "ID"
// @Param hostname query string false "主机名"
// @Param uuids query string false "uuids 用逗号分隔"
// @Success 200 {object} response.Response{data=model.PeerList}
// @Failure 500 {object} response.Response
// @Router /admin/my/peer/list [get]
// @Security token
func (ct *Peer) List(c *gin.Context) {
	query := &admin.PeerQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)

	// Also include peers from user's accessible address books (personal + shared)
	var abPeerIds []string
	service.DB.Model(&model.AddressBook{}).Where("user_id = ?", u.Id).Pluck("id", &abPeerIds)

	// Include peers from address book collections shared TO this user (personally or via group)
	var sharedIds []string
	if u.GroupId > 0 {
		service.DB.Raw(`
			SELECT ab.id FROM address_book ab
			INNER JOIN address_book_collection_rules r ON ab.collection_id = r.collection_id
			WHERE (r.type = 1 AND r.to_id = ?) OR (r.type = 2 AND r.to_id = ?)
		`, u.Id, u.GroupId).Pluck("id", &sharedIds)
	} else {
		service.DB.Raw(`
			SELECT ab.id FROM address_book ab
			INNER JOIN address_book_collection_rules r ON ab.collection_id = r.collection_id
			WHERE r.type = 1 AND r.to_id = ?
		`, u.Id).Pluck("id", &sharedIds)
	}

	// Merge both sets of peer IDs
	allIds := make(map[string]struct{})
	for _, id := range abPeerIds {
		allIds[id] = struct{}{}
	}
	for _, id := range sharedIds {
		allIds[id] = struct{}{}
	}
	mergedPeerIds := make([]string, 0, len(allIds))
	for id := range allIds {
		mergedPeerIds = append(mergedPeerIds, id)
	}

	res := service.AllService.PeerService.List(query.Page, query.PageSize, func(tx *gorm.DB) {
		if len(mergedPeerIds) > 0 {
			tx.Where("user_id = ? OR id in (?)", u.Id, mergedPeerIds)
		} else {
			tx.Where("user_id = ?", u.Id)
		}
		if query.TimeAgo > 0 {
			lt := time.Now().Unix() - int64(query.TimeAgo)
			tx.Where("last_online_time < ?", lt)
		}
		if query.TimeAgo < 0 {
			lt := time.Now().Unix() + int64(query.TimeAgo)
			tx.Where("last_online_time > ?", lt)
		}
		if query.Id != "" {
			tx.Where("id like ?", "%"+query.Id+"%")
		}
		if query.Hostname != "" {
			tx.Where("hostname like ?", "%"+query.Hostname+"%")
		}
		if query.Uuids != "" {
			tx.Where("uuid in (?)", query.Uuids)
		}
	})
	response.Success(c, res)
}
