package model

import "time"

// PayOrder 订阅支付订单（持久化待支付/已支付状态）
type PayOrder struct {
	IdModel
	// OutTradeNo 商户订单号（全局唯一，作为幂等键）
	OutTradeNo string `gorm:"uniqueIndex;size:64" json:"out_trade_no"`
	// UserID 下单用户 ID
	UserID uint `gorm:"index" json:"user_id"`
	// Plan 套餐标识
	Plan string `gorm:"size:32;default:'pro'" json:"plan"`
	// PlanKey 时长选项 key，如 1m / 3m / 6m / 12m
	PlanKey string `gorm:"size:16" json:"plan_key"`
	// AmountCents 订单金额（分）
	AmountCents int64 `json:"amount_cents"`
	// Channel 支付渠道：wechat / alipay
	Channel string `gorm:"size:16" json:"channel"`
	// PeriodDays 订阅周期天数
	PeriodDays int `json:"period_days"`
	// Status 订单状态：pending / paid / failed / closed
	Status string `gorm:"size:16;index;default:'pending'" json:"status"`
	// CashierURL 收银台链接
	CashierURL string `gorm:"size:512" json:"cashier_url"`
	// QRPayload 可选：后端直出二维码内容
	QRPayload string `gorm:"size:512" json:"qr_payload"`
	// PaidAt 支付完成时间
	PaidAt *time.Time `json:"paid_at"`
	// CallbackRaw 原始回调报文（仅用于对账/排错，不对外暴露）
	CallbackRaw string `gorm:"type:text" json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (PayOrder) TableName() string {
	return "pay_orders"
}
