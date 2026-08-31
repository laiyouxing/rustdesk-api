package model

// 账户操作审计日志：记录管理员对账户/会员的 新建、延长、终止 等操作
type AccountOpLog struct {
	IdModel
	// OperatorId 操作者（管理员）用户 ID
	OperatorId uint `json:"operator_id" gorm:"default:0;not null;index"`
	// OperatorName 操作者用户名（冗余，便于展示）
	OperatorName string `json:"operator_name" gorm:"size:64;default:'';not null;"`
	// TargetUserId 目标用户 ID
	TargetUserId uint `json:"target_user_id" gorm:"default:0;not null;index"`
	// TargetUsername 目标用户名（冗余）
	TargetUsername string `json:"target_username" gorm:"size:64;default:'';not null;"`
	// Action 操作类型：create（新建） / extend（延长） / terminate（终止）
	Action string `json:"action" gorm:"size:32;default:'';not null;index"`
	// Detail 操作详情（如 "延长 30 天 / 永久 / 终止会员"）
	Detail string `json:"detail" gorm:"size:256;default:'';not null;"`
	TimeModel
}

func (AccountOpLog) TableName() string {
	return "account_op_logs"
}

type AccountOpLogList struct {
	AccountOpLogs []*AccountOpLog `json:"list"`
	Pagination
}
