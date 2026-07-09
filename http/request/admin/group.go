package admin

import "github.com/lejianwen/rustdesk-api/v2/model"

type GroupForm struct {
	Id       uint   `json:"id"`
	Name     string `json:"name" validate:"required"`
	Type     int    `json:"type"`
	ParentId uint   `json:"parent_id"`
}

func (gf *GroupForm) FromGroup(group *model.Group) *GroupForm {
	gf.Id = group.Id
	gf.Name = group.Name
	gf.Type = group.Type
	gf.ParentId = group.ParentId
	return gf
}

func (gf *GroupForm) ToGroup() *model.Group {
	group := &model.Group{}
	group.Id = gf.Id
	group.Name = gf.Name
	group.Type = gf.Type
	group.ParentId = gf.ParentId
	return group
}

type DeviceGroupForm struct {
	Id   uint   `json:"id"`
	Name string `json:"name" validate:"required"`
}

func (gf *DeviceGroupForm) ToDeviceGroup() *model.DeviceGroup {
	group := &model.DeviceGroup{}
	group.Id = gf.Id
	group.Name = gf.Name
	return group
}
