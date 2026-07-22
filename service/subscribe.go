package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/lib/payverify"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

// SubscribeService 订阅付费服务
type SubscribeService struct {
	verifier   payverify.PaymentVerifier
	replayGuard *payverify.ReplayGuard
}

// NewSubscribeService 创建 SubscribeService
func NewSubscribeService() *SubscribeService {
	pc := Config.Payment
	var verifier payverify.PaymentVerifier
	if pc.Secret != "" && pc.MerchantID != "" {
		verifier = payverify.NewGenericProvider(pc)
	}
	rg := payverify.NewReplayGuard(pc.ReplayWindowSec, pc.TimestampField)
	return &SubscribeService{
		verifier:    verifier,
		replayGuard: rg,
	}
}

// Db 返回 DB 实例
func (s *SubscribeService) Db() *gorm.DB {
	return DB
}

// generateOutTradeNo 生成商户订单号
// 格式：SUB + YYYYMMDD + 12hex = 约 24 字符（≤32，满足多数平台长度上限）
func (s *SubscribeService) generateOutTradeNo() string {
	datePart := time.Now().Format("20060102")
	randBytes := make([]byte, 6)
	rand.Read(randBytes)
	randPart := hex.EncodeToString(randBytes)
	return "SUB" + datePart + randPart
}

// CreateOrder 创建订阅订单
func (s *SubscribeService) CreateOrder(userID uint, channel, plan string) (*model.PayOrder, error) {
	if plan == "" {
		plan = Config.Subscription.Plan
	}
	if plan == "" {
		plan = "pro"
	}
	if channel != "wechat" && channel != "alipay" {
		return nil, fmt.Errorf("unsupported channel: %s", channel)
	}

	outTradeNo := s.generateOutTradeNo()
	priceCents := Config.Subscription.PriceCents
	if priceCents <= 0 {
		priceCents = 1000 // 默认 ¥10.00
	}
	periodDays := Config.Subscription.PeriodDays
	if periodDays <= 0 {
		periodDays = 30
	}

	order := &model.PayOrder{
		OutTradeNo:  outTradeNo,
		UserID:      userID,
		Plan:        plan,
		AmountCents: priceCents,
		Channel:     channel,
		Status:      "pending",
		CashierURL:  "", // 下游平台收银台 URL，实际接入时需调用平台 API 获取
	}

	if err := s.Db().Create(order).Error; err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	return order, nil
}

// QueryOrder 查询订单状态
// 安全检查：只有订单所属用户或管理员可查询
func (s *SubscribeService) QueryOrder(outTradeNo string, userID uint) (*model.PayOrder, error) {
	order := &model.PayOrder{}
	if err := s.Db().Where("out_trade_no = ?", outTradeNo).First(order).Error; err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if order.UserID != userID {
		return nil, fmt.Errorf("order not found")
	}
	return order, nil
}

// HandleNotify 处理支付平台异步回调
// 安全流程：验签 → 防重放 → 查订单 → 幂等 → paid 才发码
func (s *SubscribeService) HandleNotify(params map[string]string) (bool, error) {
	// 1. 验签
	if s.verifier != nil {
		ok, err := s.verifier.Verify(params)
		if err != nil || !ok {
			Logger.Warnf("notify verify failed: %v", err)
			return false, fmt.Errorf("sign verify failed")
		}
	} else {
		Logger.Warn("payment verifier not configured, skip sign verification")
	}

	// 2. 防重放（时间戳校验）
	if tsField := Config.Payment.TimestampField; tsField != "" {
		if ts, ok := params[tsField]; ok && ts != "" {
			if err := s.replayGuard.Check(ts); err != nil {
				Logger.Warnf("notify replay check failed: %v", err)
				return false, fmt.Errorf("replay check failed")
			}
		}
	}

	// 3. 提取 out_trade_no
	outTradeNoField := Config.Payment.OutTradeNoField
	if outTradeNoField == "" {
		outTradeNoField = "out_trade_no"
	}
	outTradeNo, ok := params[outTradeNoField]
	if !ok || outTradeNo == "" {
		return false, fmt.Errorf("out_trade_no not found in params")
	}

	// 4. 提取交易状态
	tradeStatusField := Config.Payment.TradeStatusField
	if tradeStatusField == "" {
		tradeStatusField = "trade_status"
	}
	tradeStatus, ok := params[tradeStatusField]
	if !ok {
		// 若平台不传交易状态字段，默认认为成功
		tradeStatus = Config.Payment.PaidStatusValue
	}

	// 5. 只有已支付状态才处理
	paidStatusValue := Config.Payment.PaidStatusValue
	if paidStatusValue == "" {
		paidStatusValue = "TRADE_SUCCESS"
	}
	if tradeStatus != paidStatusValue {
		// 非支付成功回调，忽略
		Logger.Infof("notify ignored: status=%s, out_trade_no=%s", tradeStatus, outTradeNo)
		return true, nil
	}

	// 6. 事务：幂等改单 + 发码 + 激活
	now := time.Now()
	err := s.Db().Transaction(func(tx *gorm.DB) error {
		order := &model.PayOrder{}
		if err := tx.Where("out_trade_no = ?", outTradeNo).First(order).Error; err != nil {
			return fmt.Errorf("order not found: %s", outTradeNo)
		}

		// 幂等：已支付直接跳过
		if order.Status == "paid" {
			Logger.Infof("notify idempotent: order %s already paid, skip", outTradeNo)
			return nil
		}

		// 状态机校验：仅 pending 可转 paid
		if order.Status != "pending" {
			return fmt.Errorf("order %s status is %s, cannot transition to paid", outTradeNo, order.Status)
		}

		// 改单
		if err := tx.Model(order).Updates(map[string]interface{}{
			"status":       "paid",
			"paid_at":      &now,
			"callback_raw": fmt.Sprintf("%v", params),
		}).Error; err != nil {
			return err
		}
		order.Status = "paid"
		order.PaidAt = &now
		order.CallbackRaw = fmt.Sprintf("%v", params)

		// 生成邀请码并激活用户订阅
		periodDays := Config.Subscription.PeriodDays
		if periodDays <= 0 {
			periodDays = 30
		}
		plan := order.Plan
		if plan == "" {
			plan = "pro"
		}

		// 使用 InviteCodeService 生成码
		ics := &InviteCodeService{}
		ic, err := ics.Generate(plan, order.UserID, outTradeNo, periodDays)
		if err != nil {
			return fmt.Errorf("generate code: %w", err)
		}

		// 激活订阅（顺延策略）
		user := &model.User{}
		if err := tx.Where("id = ?", order.UserID).First(user).Error; err != nil {
			return err
		}
		periodDuration := time.Duration(periodDays*24) * time.Hour
		var newExpire time.Time
		if user.SubscriptionExpireAt == nil || user.SubscriptionExpireAt.Before(now) {
			newExpire = now.Add(periodDuration)
		} else {
			newExpire = user.SubscriptionExpireAt.Add(periodDuration)
		}
		if err := tx.Model(&model.User{}).Where("id = ?", order.UserID).
			Updates(map[string]interface{}{
				"subscription_plan":      plan,
				"subscription_expire_at": &newExpire,
			}).Error; err != nil {
			return err
		}

		// 更新码状态
		icTime := now
		if err := tx.Model(&model.InviteCode{}).Where("id = ?", ic.Id).
			Updates(map[string]interface{}{
				"status":   "used",
				"used_by":  order.UserID,
				"used_at":  &icTime,
				"expire_at": newExpire,
			}).Error; err != nil {
			return err
		}

		Logger.Infof("subscribe activated: user=%d, order=%s, plan=%s, expire=%s",
			order.UserID, outTradeNo, plan, newExpire.Format(time.RFC3339))
		return nil
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// ClaimCode 订单号认领邀请码（兜底：用户已支付但未收到码）
// 条件：订单已 paid 且未生成过邀请码（尚未兑换）
func (s *SubscribeService) ClaimCode(userID uint, outTradeNo string) (*model.InviteCode, error) {
	order := &model.PayOrder{}
	if err := s.Db().Where("out_trade_no = ? AND user_id = ?", outTradeNo, userID).First(order).Error; err != nil {
		return nil, fmt.Errorf("ORDER_NOT_FOUND")
	}
	if order.Status != "paid" {
		return nil, fmt.Errorf("ORDER_NOT_PAID")
	}

	// 检查是否已生成过邀请码
	ics := &InviteCodeService{}
	existing := ics.InfoByOrderID(outTradeNo)
	if existing != nil && existing.Id > 0 {
		return existing, nil
	}

	// 生成新邀请码并激活
	periodDays := Config.Subscription.PeriodDays
	if periodDays <= 0 {
		periodDays = 30
	}
	plan := order.Plan
	if plan == "" {
		plan = "pro"
	}

	ic, err := ics.Generate(plan, userID, outTradeNo, periodDays)
	if err != nil {
		return nil, fmt.Errorf("generate code: %w", err)
	}

	// 激活
	_, err = ics.Activate(ic.Code, userID)
	if err != nil {
		return nil, fmt.Errorf("activate code: %w", err)
	}

	return ic, nil
}

// RedeemCode 用户兑换邀请码（手动发放的邀请码）
func (s *SubscribeService) RedeemCode(userID uint, codeStr string) (*model.InviteCode, error) {
	ics := &InviteCodeService{}
	ic, err := ics.Activate(codeStr, userID)
	if err != nil {
		errMsg := err.Error()
		// 将错误映射为统一错误码
		switch {
		case contains(errMsg, "code not found"):
			return nil, fmt.Errorf("CODE_NOT_FOUND")
		case contains(errMsg, "code already used"):
			return nil, fmt.Errorf("CODE_USED")
		case contains(errMsg, "code revoked"):
			return nil, fmt.Errorf("CODE_REVOKED")
		case contains(errMsg, "code expired"):
			return nil, fmt.Errorf("CODE_EXPIRED")
		default:
			return nil, err
		}
	}
	return ic, nil
}

// GetMine 获取当前用户的订阅信息
func (s *SubscribeService) GetMine(userID uint) (*model.User, error) {
	user := &model.User{}
	if err := s.Db().Where("id = ?", userID).First(user).Error; err != nil {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}
