package model

// 地址簿操作审计日志：记录地址簿的 新建/编辑/删除/批量/标签/分组/规则/分享 等操作
type AddressBookOpLog struct {
	IdModel
	// OperatorId 操作者（管理员或用户自身）ID
	OperatorId uint `json:"operator_id" gorm:"default:0;not null;index"`
	// OperatorName 操作者用户名
	OperatorName string `json:"operator_name" gorm:"size:64;default:'';not null;"`
	// UserId 所属用户 ID（地址簿所有者）
	UserId uint `json:"user_id" gorm:"default:0;not null;index"`
	// Username 所属用户名
	Username string `json:"username" gorm:"size:64;default:'';not null;"`
	// Action 操作类型：create/update/delete/batch_create/update_tags/
	// share/tag_create/tag_update/tag_delete/
	// collection_create/collection_update/collection_delete/
	// rule_create/rule_update/rule_delete
	Action string `json:"action" gorm:"size:32;default:'';not null;index"`
	// Detail 操作详情（如 "添加地址簿 192.168.1.100"）
	Detail string `json:"detail" gorm:"size:256;default:'';not null;"`
	TimeModel
}

func (AddressBookOpLog) TableName() string {
	return "address_book_op_logs"
}

type AddressBookOpLogList struct {
	AddressBookOpLogs []*AddressBookOpLog `json:"list"`
	Pagination
}
