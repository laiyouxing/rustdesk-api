package service

import (
	"errors"
	"math/rand"
	"strconv"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type PeerService struct {
}

// FindById 根据id查找
func (ps *PeerService) FindById(id string) *model.Peer {
	p := &model.Peer{}
	DB.Where("id = ?", id).First(p)
	return p
}
func (ps *PeerService) FindByUuid(uuid string) *model.Peer {
	p := &model.Peer{}
	DB.Where("uuid = ?", uuid).First(p)
	return p
}
func (ps *PeerService) InfoByRowId(id uint) *model.Peer {
	p := &model.Peer{}
	DB.Where("row_id = ?", id).First(p)
	return p
}

// FindByUserIdAndUuid 根据用户id和uuid查找peer
func (ps *PeerService) FindByUserIdAndUuid(uuid string, userId uint) *model.Peer {
	p := &model.Peer{}
	DB.Where("uuid = ? and user_id = ?", uuid, userId).First(p)
	return p
}

// UuidBindUserId 绑定用户id
func (ps *PeerService) UuidBindUserId(deviceId string, uuid string, userId uint) {
	peer := ps.FindByUuid(uuid)
	// 如果存在则更新
	if peer.RowId > 0 {
		peer.UserId = userId
		ps.Update(peer)
	} else {
		// 不存在则创建
		/*if deviceId != "" {
			DB.Create(&model.Peer{
				Id:     deviceId,
				Uuid:   uuid,
				UserId: userId,
			})
		}*/
	}
}

// UuidUnbindUserId 解绑用户id, 用于用户注销
func (ps *PeerService) UuidUnbindUserId(uuid string, userId uint) {
	peer := ps.FindByUserIdAndUuid(uuid, userId)
	if peer.RowId > 0 {
		DB.Model(peer).Update("user_id", 0)
	}
}

// EraseUserId 清除用户id, 用于用户删除
func (ps *PeerService) EraseUserId(userId uint) error {
	return DB.Model(&model.Peer{}).Where("user_id = ?", userId).Update("user_id", 0).Error
}

// ListByUserIds 根据用户id取列表
func (ps *PeerService) ListByUserIds(userIds []uint, page, pageSize uint) (res *model.PeerList) {
	res = &model.PeerList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Peer{})
	tx.Where("user_id in (?)", userIds)
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Peers)
	return
}

func (ps *PeerService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.PeerList) {
	res = &model.PeerList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Peer{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Peers)
	return
}

// ListFilterByUserId 根据用户id过滤Peer列表
func (ps *PeerService) ListFilterByUserId(page, pageSize uint, where func(tx *gorm.DB), userId uint) (res *model.PeerList) {
	userWhere := func(tx *gorm.DB) {
		tx.Where("user_id = ?", userId)
		// 如果还有额外的筛选条件，执行它
		if where != nil {
			where(tx)
		}
	}
	return ps.List(page, pageSize, userWhere)
}

// Create 创建
func (ps *PeerService) Create(u *model.Peer) error {
	res := DB.Create(u).Error
	return res
}

// Delete 删除, 同时也应该删除token
func (ps *PeerService) Delete(u *model.Peer) error {
	uuid := u.Uuid
	err := DB.Delete(u).Error
	if err != nil {
		return err
	}
	// 删除token
	return AllService.UserService.FlushTokenByUuid(uuid)
}

// GetUuidListByIDs 根据ids获取uuid列表
func (ps *PeerService) GetUuidListByIDs(ids []uint) ([]string, error) {
	var uuids []string
	err := DB.Model(&model.Peer{}).
		Where("row_id in (?)", ids).
		Pluck("uuid", &uuids).Error
	//过滤uuids中的空字符串
	var newUuids []string
	for _, uuid := range uuids {
		if uuid != "" {
			newUuids = append(newUuids, uuid)
		}
	}
	return newUuids, err
}

// BatchDelete 批量删除, 同时也应该删除token
func (ps *PeerService) BatchDelete(ids []uint) error {
	uuids, err := ps.GetUuidListByIDs(ids)
	err = DB.Where("row_id in (?)", ids).Delete(&model.Peer{}).Error
	if err != nil {
		return err
	}
	// 删除token
	return AllService.UserService.FlushTokenByUuids(uuids)
}

// Update 更新
func (ps *PeerService) Update(u *model.Peer) error {
	return DB.Model(u).Updates(u).Error
}

// ClaimId 为新设备分配或取回远程 ID。
//
// 规则（已与用户确认）：
//   - ID 绑定"这台机器"(uuid)，换账号登录也不变；
//   - 已经分配过 ID 的机器直接返回原值，**不重新分配**（幂等）；
//   - 老机器本地已有 ID，走 sysinfo 上报，不会走到这里；
//   - 只有新装设备（本地没有 ID）在登录后才会触发分配。
//
// 返回的 ID 为 9 位数字，保证在 Peer 表中不重复。
func (ps *PeerService) ClaimId(uuid string, userId uint, hostname string) (string, error) {
	if uuid == "" {
		return "", errors.New("uuid 不能为空")
	}
	if p := ps.FindByUuid(uuid); p.RowId != 0 {
		// 已存在这台机器的记录
		if p.Id != "" {
			// 已有 ID：幂等返回，顺手补齐归属与主机名
			upd := map[string]interface{}{}
			if p.UserId == 0 && userId != 0 {
				upd["user_id"] = userId
			}
			if hostname != "" && hostname != p.Hostname {
				upd["hostname"] = hostname
			}
			if len(upd) > 0 {
				if err := DB.Model(&model.Peer{}).Where("row_id = ?", p.RowId).Updates(upd).Error; err != nil {
					return "", err
				}
			}
			return p.Id, nil
		}
		// 有设备记录但还没有 ID（未登录时上报过）：现在分配
		id, err := ps.allocId()
		if err != nil {
			return "", err
		}
		upd := map[string]interface{}{"id": id}
		if userId != 0 {
			upd["user_id"] = userId
		}
		if hostname != "" {
			upd["hostname"] = hostname
		}
		if err := DB.Model(&model.Peer{}).Where("row_id = ?", p.RowId).Updates(upd).Error; err != nil {
			return "", err
		}
		return id, nil
	}
	// 全新设备
	id, err := ps.allocId()
	if err != nil {
		return "", err
	}
	if err := ps.Create(&model.Peer{Id: id, Uuid: uuid, UserId: userId, Hostname: hostname}); err != nil {
		return "", err
	}
	return id, nil
}

// allocId 生成一个未被占用的 9 位数字 ID（100000000 ~ 999999999）。
// 撞号则重试；连续失败返回错误，避免死循环。
func (ps *PeerService) allocId() (string, error) {
	const maxRetry = 32
	for i := 0; i < maxRetry; i++ {
		id := strconv.Itoa(100000000 + rand.Intn(900000000))
		if ps.FindById(id).RowId == 0 {
			return id, nil
		}
	}
	return "", errors.New("分配远程 ID 失败：连续撞号")
}
