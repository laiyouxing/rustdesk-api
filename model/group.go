package model

const (
	GroupTypeDefault = 1 // 默认
	GroupTypeShare   = 2 // 共享
)

type Group struct {
	IdModel
	Name     string `json:"name" gorm:"default:'';not null;"`
	Type     int    `json:"type" gorm:"default:1;not null;"`
	ParentId uint   `json:"parent_id" gorm:"default:0;not null;index;comment:上级部门ID，0为根部门"`
	TimeModel
}

type GroupList struct {
	Groups []*Group `json:"list"`
	Pagination
}

// GroupTree 部门树节点，用于组织架构展示
type GroupTree struct {
	*Group
	Children  []*GroupTree `json:"children"`
	UserCount int64        `json:"user_count"`
}

type DeviceGroup struct {
	IdModel
	Name string `json:"name" gorm:"default:'';not null;"`
	TimeModel
}

type DeviceGroupList struct {
	DeviceGroups []*DeviceGroup `json:"list"`
	Pagination
}
