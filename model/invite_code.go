package model

import "time"

// InviteCode 订阅邀请码（自动发码/认领/手动生成共用）
// 注意区别于 model.Invitation（用于注册邀请），InviteCode 用于订阅激活。
type InviteCode struct {
	IdModel
	// Code 24 位 base62 随机串，唯一索引
	Code string `gorm:"uniqueIndex;size:64" json:"code"`
	// Plan 可兑换的套餐标识
	Plan string `gorm:"size:32;default:'pro'" json:"plan"`
	// ExpireDays 有效天数（自动发码 = 订阅周期；手动生成由管理员指定）
	ExpireDays int `json:"expire_days"`
	// ExpireAt 码本身的过期时间
	ExpireAt time.Time `gorm:"index" json:"expire_at"`
	// Status 码状态：unused / used / revoked
	Status string `gorm:"size:16;index;default:'unused'" json:"status"`
	// UsedBy 使用者用户 ID
	UsedBy uint `gorm:"index;default:0" json:"used_by"`
	// BoundOrderID 关联的商户订单号
	BoundOrderID string `gorm:"size:64;index" json:"bound_order_id"`
	// UsedAt 使用时间
	UsedAt *time.Time `json:"used_at"`
	// RevokedAt 失效时间
	RevokedAt *time.Time `json:"revoked_at"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名
func (InviteCode) TableName() string {
	return "invite_codes"
}
