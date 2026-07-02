package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
)

// ClientDownloadService 客户端下载链接管理
type ClientDownloadService struct{}

// ActiveList 获取启用的下载列表（公开）
func (s *ClientDownloadService) ActiveList() []*model.ClientDownload {
	var list []*model.ClientDownload
	DB.Where("status = ?", model.COMMON_STATUS_ENABLE).
		Order("sort_order asc, created_at desc").
		Find(&list)
	return list
}

// List 获取下载列表（管理端，分页）
func (s *ClientDownloadService) List(page, pageSize uint) ([]*model.ClientDownload, int64) {
	var list []*model.ClientDownload
	var total int64
	DB.Model(&model.ClientDownload{}).Count(&total)
	DB.Order("sort_order asc, created_at desc").Scopes(Paginate(page, pageSize)).Find(&list)
	return list, total
}

// Create 创建下载链接
func (s *ClientDownloadService) Create(v *model.ClientDownload) {
	DB.Create(v)
}

// Update 更新下载链接
func (s *ClientDownloadService) Update(v *model.ClientDownload) {
	DB.Model(v).Where("id = ?", v.Id).Updates(v)
}

// Delete 删除下载链接
func (s *ClientDownloadService) Delete(id uint) {
	DB.Delete(&model.ClientDownload{}, id)
}

// FindById 根据ID查找
func (s *ClientDownloadService) FindById(id uint) *model.ClientDownload {
	var v model.ClientDownload
	DB.Where("id = ?", id).First(&v)
	if v.Id > 0 {
		return &v
	}
	return nil
}
