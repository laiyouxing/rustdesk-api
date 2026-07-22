package payverify

import "github.com/lejianwen/rustdesk-api/v2/config"

// 平台预设辅助函数
// 本文件仅提供配置驱动的辅助方法；具体平台字段名/算法等均在 config.yaml 中配置。

// PlatformPreset 平台预设信息（用于注释参考，非执行逻辑）
type PlatformPreset struct {
	Name             string // 平台名称
	SignField        string // 签名字段
	TimestampField   string // 时间戳字段
	OutTradeNoField  string // 订单号字段
	TradeStatusField string // 交易状态字段
	PaidStatusValue  string // 已支付状态值
	SignAlgo         string // 签名算法
}

// Presets 常见平台预设（仅作为配置参考，不自动应用）
// 最终字段名以 config.yaml 中的配置为准。
//
// 码支付（codepay）:
//
//	SignField: "sign", TimestampField: "pay_no" 不直接提供时间戳,
//	鉴权机制特殊，需参阅码支付文档。建议配置 replay_window_sec: 0 关闭防重放。
//
// 易支付（epay）:
//
//	SignField: "sign", TimestampField: "notify_time" (可选字段),
//	OutTradeNoField: "out_trade_no", TradeStatusField: "trade_status",
//	PaidStatusValue: "TRADE_SUCCESS", SignAlgo: "md5"
//	签名方式: 按参数名排序，拼接 key=value&key=value...&key=商户密钥 再 MD5。
//	注：时间戳非必传，建议开启 notify_id 防重放（若平台支持），或 replay_window_sec: 0。
//
// PayJS:
//
//	SignField: "sign", TimestampField: "sPayMark" (部分版本),
//	OutTradeNoField: "out_trade_no", TradeStatusField: "return_code",
//	PaidStatusValue: "1", SignAlgo: "md5"
//
// 通用配置参考（适用于大多数仿微信/支付宝接口的平台）:
//
//	payment:
//	  merchant_id: "your_merchant_id"
//	  secret: "your_secret_key"
//	  notify_url: "https://your.domain/api/v1/subscribe/notify"
//	  sign_algo: "md5"
//	  sign_field: "sign"
//	  timestamp_field: "timestamp"
//	  replay_window_sec: 300
//	  success_keyword: "success"
//	  out_trade_no_field: "out_trade_no"
//	  trade_status_field: "trade_status"
//	  paid_status_value: "TRADE_SUCCESS"
func PresetConfig(name string) *config.PaymentConfig {
	// 本函数仅用于文档参考，不自动注入配置。
	// 请直接在 config.yaml 中填写实际平台的配置值。
	return nil
}
