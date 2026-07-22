package config

// PaymentConfig 支付平台配置（通用验签框架，配置驱动，不写死平台）
type PaymentConfig struct {
	// MerchantID 商户号 / 应用 ID
	MerchantID string `mapstructure:"merchant_id" yaml:"merchant_id"`
	// Secret 商户密钥（签名密钥）
	Secret string `mapstructure:"secret" yaml:"secret"`
	// NotifyURL 异步回调通知地址
	NotifyURL string `mapstructure:"notify_url" yaml:"notify_url"`
	// SignAlgo 签名算法：md5 | sha256 | hmac-sha256
	SignAlgo string `mapstructure:"sign_algo" yaml:"sign_algo"`
	// SignField 回调中签名字段名
	SignField string `mapstructure:"sign_field" yaml:"sign_field"`
	// TimestampField 回调中时间戳字段名
	TimestampField string `mapstructure:"timestamp_field" yaml:"timestamp_field"`
	// ReplayWindowSec 防重放窗口（秒），超出该窗口的请求拒绝
	ReplayWindowSec int `mapstructure:"replay_window_sec" yaml:"replay_window_sec"`
	// SuccessKeyword 回调成功时向平台返回的字符串（如 "success" 或 '{"code":"success"}'）
	SuccessKeyword string `mapstructure:"success_keyword" yaml:"success_keyword"`
	// OutTradeNoField 回调中订单号字段名
	OutTradeNoField string `mapstructure:"out_trade_no_field" yaml:"out_trade_no_field"`
	// TradeStatusField 回调中交易状态字段名
	TradeStatusField string `mapstructure:"trade_status_field" yaml:"trade_status_field"`
	// PaidStatusValue 回调中表示已支付的状态值（如 "TRADE_SUCCESS"）
	PaidStatusValue string `mapstructure:"paid_status_value" yaml:"paid_status_value"`
}

// SubscriptionConfig 订阅套餐配置
type SubscriptionConfig struct {
	// Plan 套餐标识
	Plan string `mapstructure:"plan" yaml:"plan"`
	// PriceCents 价格（分），如 1000 = ¥10.00
	PriceCents int64 `mapstructure:"price_cents" yaml:"price_cents"`
	// PeriodDays 订阅周期天数
	PeriodDays int `mapstructure:"period_days" yaml:"period_days"`
	// RemindDays 临期提醒天数列表（前端用）
	RemindDays []int `mapstructure:"remind_days" yaml:"remind_days"`
}
