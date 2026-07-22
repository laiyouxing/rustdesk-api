package api

// CreateOrderReq 创建订单请求
type CreateOrderReq struct {
	Channel string `json:"channel" binding:"required,oneof=wechat alipay"` // 支付渠道
	// PlanKey 时长选项 key（1m / 3m / 6m / 12m），服务端根据 key 查价
	PlanKey string `json:"plan_key" binding:"required,max=16"`
}

// ClaimReq 订单号认领请求
type ClaimReq struct {
	OutTradeNo string `json:"out_trade_no" binding:"required,max=64"` // 商户订单号
}

// RedeemReq 兑换邀请码请求
type RedeemReq struct {
	Code string `json:"code" binding:"required,max=64"` // 邀请码
}

// AdminCreateCodeReq 后台手动生成邀请码请求
type AdminCreateCodeReq struct {
	Plan       string `json:"plan"`        // 套餐标识，缺省 "pro"
	ExpireDays int    `json:"expire_days"` // 有效天数，缺省 30
}
