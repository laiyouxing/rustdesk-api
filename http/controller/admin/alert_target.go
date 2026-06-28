package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type AlertTargetCtl struct {
}

func (c *AlertTargetCtl) List(ctx *gin.Context) {
	alertId := ctx.Query("alert_id")
	if alertId == "" {
		response.Fail(ctx, 101, "alert_id required")
		return
	}
	var targets []model.AlertTarget
	service.DB.Where("alert_id = ?", alertId).Find(&targets)
	response.Success(ctx, gin.H{"list": targets})
}

func (c *AlertTargetCtl) Create(ctx *gin.Context) {
	f := &struct {
		AlertId    uint   `json:"alert_id"`
		TargetType string `json:"target_type"`
		TargetId   string `json:"target_id"`
		TargetName string `json:"target_name"`
	}{}
	if err := ctx.ShouldBindJSON(f); err != nil {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if f.AlertId == 0 || f.TargetType == "" || f.TargetId == "" {
		response.Fail(ctx, 101, "参数不完整")
		return
	}
	t := &model.AlertTarget{
		AlertId:    f.AlertId,
		TargetType: f.TargetType,
		TargetId:   f.TargetId,
		TargetName: f.TargetName,
	}
	service.DB.Create(t)
	response.Success(ctx, t)
}

func (c *AlertTargetCtl) Delete(ctx *gin.Context) {
	form := &struct{ Id uint `json:"id"` }{}
	if err := ctx.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(ctx, 101, "ID不能为空")
		return
	}
	service.DB.Delete(&model.AlertTarget{}, form.Id)
	response.Success(ctx, nil)
}

// AvailableCollections returns collections accessible by current user
func (c *AlertTargetCtl) AvailableCollections(ctx *gin.Context) {
	user := ctx.MustGet("curUser").(*model.User)
	if user == nil || user.Id == 0 {
		response.Fail(ctx, 101, "未登录")
		return
	}

	type collectionInfo struct {
		Id       uint   `json:"id"`
		Name     string `json:"name"`
		OwnerId  uint   `json:"owner_id"`
		OwnerName string `json:"owner_name"`
		PeerCount int64  `json:"peer_count"`
	}

	var result []collectionInfo

	// 1. Own collections
	var ownColls []model.AddressBookCollection
	service.DB.Where("user_id = ?", user.Id).Find(&ownColls)
	for _, c := range ownColls {
		var cnt int64
		service.DB.Model(&model.AddressBook{}).Where("collection_id = ?", c.Id).Count(&cnt)
		result = append(result, collectionInfo{
			Id:       c.Id,
			Name:     c.Name + " (我的)",
			OwnerId:  user.Id,
			OwnerName: user.Username,
			PeerCount: cnt,
		})
	}

	// 2. Collections shared to me (personally or via group)
	var rules []model.AddressBookCollectionRule
	ruleQuery := service.DB.Where("type = ? AND to_id = ?",
		model.ShareAddressBookRuleTypePersonal, user.Id)
	if user.GroupId > 0 {
		ruleQuery = service.DB.Where(
			"(type = ? AND to_id = ?) OR (type = ? AND to_id = ?)",
			model.ShareAddressBookRuleTypePersonal, user.Id,
			model.ShareAddressBookRuleTypeGroup, user.GroupId,
		)
	}
	ruleQuery.Find(&rules)

	for _, rule := range rules {
		col := &model.AddressBookCollection{}
		service.DB.First(col, rule.CollectionId)
		if col.Id == 0 || col.UserId == user.Id {
			continue // skip own (already added)
		}
		var cnt int64
		service.DB.Model(&model.AddressBook{}).Where("collection_id = ?", col.Id).Count(&cnt)
		owner := &model.User{}
		service.DB.First(owner, col.UserId)
		ownerName := owner.Username
		if ownerName == "" {
			ownerName = "未知"
		}
		result = append(result, collectionInfo{
			Id:        col.Id,
			Name:      col.Name + " (来自 " + ownerName + ")",
			OwnerId:   col.UserId,
			OwnerName: ownerName,
			PeerCount: cnt,
		})
	}

	response.Success(ctx, gin.H{"list": result})
}

// AvailablePeers returns peers in a given collection
func (c *AlertTargetCtl) AvailablePeers(ctx *gin.Context) {
	collectionId := ctx.Query("collection_id")
	if collectionId == "" {
		response.Fail(ctx, 101, "collection_id required")
		return
	}

	var abEntries []model.AddressBook
	service.DB.Where("collection_id = ?", collectionId).Find(&abEntries)

	var peers []map[string]interface{}
	for _, ab := range abEntries {
		p := service.AllService.PeerService.FindById(ab.Id)
		hostname := ab.Alias
		if hostname == "" && p != nil {
			hostname = p.Hostname
		}
		if hostname == "" {
			hostname = ab.Id
		}
		peers = append(peers, map[string]interface{}{
			"peer_id":  ab.Id,
			"hostname": hostname,
		})
	}
	response.Success(ctx, gin.H{"list": peers})
}
