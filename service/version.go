package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
)

// AppReleaseService 版本发布管理
type AppReleaseService struct {
}

func (s *AppReleaseService) Latest(platform string) *model.AppRelease {
	var v model.AppRelease
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

func (s *AppReleaseService) List(page, pageSize uint) ([]*model.AppRelease, int64) {
	var list []*model.AppRelease
	var total int64
	DB.Model(&model.AppRelease{}).Count(&total)
	DB.Order("created_at desc").Scopes(Paginate(page, pageSize)).Find(&list)
	return list, total
}

func (s *AppReleaseService) Create(v *model.AppRelease) {
	DB.Create(v)
}

func (s *AppReleaseService) Update(v *model.AppRelease) {
	DB.Model(v).Where("id = ?", v.Id).Updates(v)
}

func (s *AppReleaseService) Delete(id uint) {
	DB.Delete(&model.AppRelease{}, id)
}

func (s *AppReleaseService) FindById(id uint) *model.AppRelease {
	var v model.AppRelease
	DB.Where("id = ?", id).First(&v)
	if v.Id > 0 {
		return &v
	}
	return nil
}
