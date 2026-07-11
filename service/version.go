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
		// platform 别名兼容：客户端可能发送 ubuntu/linux、macos/mac，
		// 而后台存储的 platform 取值可能不一致，故按同义组匹配。
		aliasMap := map[string][]string{
			"ubuntu":  {"ubuntu", "linux"},
			"linux":   {"linux", "ubuntu"},
			"macos":   {"macos", "mac"},
			"mac":     {"mac", "macos"},
			"windows": {"windows"},
		}
		platforms, ok := aliasMap[platform]
		if !ok {
			platforms = []string{platform}
		}
		db = db.Where("platform IN ?", platforms)
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

func (s *AppReleaseService) Create(v *model.AppRelease) error {
	return DB.Create(v).Error
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
