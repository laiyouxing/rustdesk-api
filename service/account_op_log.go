package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

// AccountOpLogService 账户操作审计日志服务
type AccountOpLogService struct {
}

// Log 记录一条账户操作审计日志
func (s *AccountOpLogService) Log(operatorId uint, operatorName string, targetUserId uint, targetUsername, action, detail string) error {
	l := &model.AccountOpLog{
		OperatorId:    operatorId,
		OperatorName:  operatorName,
		TargetUserId:  targetUserId,
		TargetUsername: targetUsername,
		Action:        action,
		Detail:        detail,
	}
	return DB.Create(l).Error
}

// List 分页查询
func (s *AccountOpLogService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AccountOpLogList) {
	res = &model.AccountOpLogList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AccountOpLog{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Order("id desc").Find(&res.AccountOpLogs)
	if res.AccountOpLogs == nil {
		res.AccountOpLogs = []*model.AccountOpLog{}
	}
	return
}
