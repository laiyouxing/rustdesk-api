package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

// AddressBookOpLogService 地址簿操作审计日志服务
type AddressBookOpLogService struct {
}

// Log 记录一条地址簿操作审计日志
func (s *AddressBookOpLogService) Log(operatorId uint, operatorName string, userId uint, username, action, detail string) error {
	l := &model.AddressBookOpLog{
		OperatorId:    operatorId,
		OperatorName:  operatorName,
		UserId:        userId,
		Username:      username,
		Action:        action,
		Detail:        detail,
	}
	return DB.Create(l).Error
}

// List 分页查询
func (s *AddressBookOpLogService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AddressBookOpLogList) {
	res = &model.AddressBookOpLogList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AddressBookOpLog{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Order("id desc").Find(&res.AddressBookOpLogs)
	if res.AddressBookOpLogs == nil {
		res.AddressBookOpLogs = []*model.AddressBookOpLog{}
	}
	return
}
