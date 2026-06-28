package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
)

type VersionService struct {
}

func (s *VersionService) Latest(platform string) *model.Version {
	var v model.Version
	db := DB.Where("status = ?", model.COMMON_STATUS_ENABLE)
	if platform != "" {
		db = db.Where("platform = ?", platform)
	}
	db.Order("created_at desc").First(&v)
	if v.Id > 0 {
		return &v
	}
	return nil
}

func (s *VersionService) List(page, pageSize uint) ([]*model.Version, int64) {
	var list []*model.Version
	var total int64
	DB.Model(&model.Version{}).Count(&total)
	DB.Order("created_at desc").Scopes(Paginate(page, pageSize)).Find(&list)
	return list, total
}

func (s *VersionService) Create(v *model.Version) {
	DB.Create(v)
}

func (s *VersionService) Update(v *model.Version) {
	DB.Model(v).Where("id = ?", v.Id).Updates(v)
}

func (s *VersionService) Delete(id uint) {
	DB.Delete(&model.Version{}, id)
}

func (s *VersionService) FindById(id uint) *model.Version {
	var v model.Version
	DB.Where("id = ?", id).First(&v)
	if v.Id > 0 {
		return &v
	}
	return nil
}
