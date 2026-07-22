package config

// PaymentConfig 码支付配置（自我实现，无外部平台依赖）
type PaymentConfig struct {
	// Enable 是否启用码支付
	Enable bool `mapstructure:"enable" yaml:"enable"`
	// SecretKey 签名密钥（用于回调通知验签）
	SecretKey string `mapstructure:"secret_key" yaml:"secret_key"`
	// NotifyURL 异步回调通知地址（支付确认工具调用）
	NotifyURL string `mapstructure:"notify_url" yaml:"notify_url"`
	// OrderExpireSec 订单未支付自动关闭时间（秒），默认 600
	OrderExpireSec int `mapstructure:"order_expire_sec" yaml:"order_expire_sec"`

	// Cashier 收银台展示配置
	Cashier CashierConfig `mapstructure:"cashier" yaml:"cashier"`
}

// CashierConfig 收银台展示
type CashierConfig struct {
	// SiteName 站点名称，在收银台页面上方显示
	SiteName string `mapstructure:"site_name" yaml:"site_name"`
	// AlipayQR 支付宝收款码图片路径（相对 resources/ 或绝对路径）
	AlipayQR string `mapstructure:"alipay_qr" yaml:"alipay_qr"`
	// WechatQR 微信收款码图片路径
	WechatQR string `mapstructure:"wechat_qr" yaml:"wechat_qr"`
	// MonitorTip 收款确认方式的提示文案
	MonitorTip string `mapstructure:"monitor_tip" yaml:"monitor_tip"`
}

// PlanOption 套餐时长选项
type PlanOption struct {
	// Key 标识键，前端传此值
	Key string `mapstructure:"key" yaml:"key"`
	// Name 显示名称，如"1个月"、"3个月"
	Name string `mapstructure:"name" yaml:"name"`
	// PriceCents 价格（分）
	PriceCents int64 `mapstructure:"price_cents" yaml:"price_cents"`
	// PeriodDays 订阅周期天数
	PeriodDays int `mapstructure:"period_days" yaml:"period_days"`
}

// SubscriptionConfig 订阅套餐配置
type SubscriptionConfig struct {
	// Plan 套餐标识
	Plan string `mapstructure:"plan" yaml:"plan"`
	// Plans 可选时长列表
	Plans []PlanOption `mapstructure:"plans" yaml:"plans"`
	// PriceCents 兼容旧配置，单一定价时使用
	PriceCents int64 `mapstructure:"price_cents" yaml:"price_cents"`
	// PeriodDays 兼容旧配置，单一时长时使用
	PeriodDays int `mapstructure:"period_days" yaml:"period_days"`
	// RemindDays 临期提醒天数列表（前端用）
	RemindDays []int `mapstructure:"remind_days" yaml:"remind_days"`
}

// LookupPlan 按 key 查找 PlanOption，找不到返回 nil
func (sc *SubscriptionConfig) LookupPlan(key string) *PlanOption {
	if key == "" && len(sc.Plans) > 0 {
		return &sc.Plans[0]
	}
	for _, p := range sc.Plans {
		if p.Key == key {
			return &p
		}
	}
	return nil
}
