package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type StrategyService struct {
}

func (s *StrategyService) InfoById(id uint) *model.Strategy {
	u := &model.Strategy{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (s *StrategyService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.StrategyList) {
	res = &model.StrategyList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Strategy{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Strategies)
	return
}

func (s *StrategyService) Create(u *model.Strategy) error {
	res := DB.Create(u).Error
	return res
}

func (s *StrategyService) Update(u *model.Strategy) error {
	return DB.Model(u).Select("*").Omit("created_at").Updates(u).Error
}

func (s *StrategyService) Delete(u *model.Strategy) error {
	return DB.Delete(u).Error
}
