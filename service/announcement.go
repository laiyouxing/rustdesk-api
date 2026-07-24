package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
)

type AnnouncementService struct {
}

func (as *AnnouncementService) List(where ...interface{}) *[]model.Announcement {
	announcements := &[]model.Announcement{}
	DB.Where("status = 1").Order("created_at desc").Find(announcements, where...)
	return announcements
}

// ListActiveForClient 返回客户端可显示的公告列表
func (as *AnnouncementService) ListActiveForClient() *[]map[string]interface{} {
	announcements := &[]model.Announcement{}
	DB.Where("status = 1").Order("created_at desc").Find(announcements)
	result := &[]map[string]interface{}{}
	for _, a := range *announcements {
		*result = append(*result, map[string]interface{}{
			"id":         a.Id,
			"title":      a.Title,
			"content":    a.Content,
			"created_at": a.CreatedAt,
		})
	}
	return result
}

func (as *AnnouncementService) Info(id int) *model.Announcement {
	a := &model.Announcement{}
	DB.Where("id = ?", id).First(a)
	return a
}

func (as *AnnouncementService) Create(a *model.Announcement) {
	DB.Create(a)
}

func (as *AnnouncementService) Update(a *model.Announcement) {
	DB.Model(a).Updates(a)
}

func (as *AnnouncementService) Delete(a *model.Announcement) {
	DB.Delete(a)
}
