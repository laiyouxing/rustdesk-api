package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/lib/payverify"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

// SubscribeService 订阅付费服务
type SubscribeService struct{}

// NewSubscribeService 创建 SubscribeService
func NewSubscribeService() *SubscribeService {
	return &SubscribeService{}
}

// Db 返回 DB 实例
func (s *SubscribeService) Db() *gorm.DB {
	return DB
}

// generateOutTradeNo 生成商户订单号
// 格式：SUB + YYYYMMDD + 12hex = 约 24 字符
func (s *SubscribeService) generateOutTradeNo() string {
	datePart := time.Now().Format("20060102")
	b := make([]byte, 6)
	rand.Read(b)
	return "SUB" + datePart + hex.EncodeToString(b)
}

// PayConfig 返回支付配置的快捷引用
func (s *SubscribeService) PayConfig() (secretKey string, expireSec int) {
	pc := Config.Payment
	return pc.SecretKey, pc.OrderExpireSec
}

// ExtractAmountFromSMS 从短信内容中提取支付金额（元）
// 支持格式: "到账10.00元"、"收款10元"、"到账10.5元"等
func (s *SubscribeService) ExtractAmountFromSMS(msg string) (string, error) {
	// 匹配 "到账/收款 + 数字.数字 + 元" 模式
	re := regexp.MustCompile(`(?:到账|收款|入账|收到)[：:\s]*(\d+\.?\d*)\s*元`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return "", fmt.Errorf("cannot extract amount from SMS: %s", msg)
	}
	amount := matches[1]
	// 补全小数位
	if !strings.Contains(amount, ".") {
		amount += ".00"
	}
	return amount, nil
}

// MatchOrderByAmount 按金额匹配最近未支付的订单（SmsForwarder 按金额回调用）
// amount: 字符串金额，支持 "10.00" 或 "10" 格式
// 返回匹配到的 out_trade_no
func (s *SubscribeService) MatchOrderByAmount(amount string) (string, error) {
	// 解析金额到分
	amount = strings.TrimSpace(amount)
	var amountCents int64
	if idx := strings.Index(amount, "."); idx >= 0 {
		intPart := amount[:idx]
		decPart := amount[idx+1:]
		if len(decPart) > 2 {
			decPart = decPart[:2]
		}
		for len(decPart) < 2 {
			decPart += "0"
		}
		ai, _ := strconv.ParseInt(intPart, 10, 64)
		ad, _ := strconv.ParseInt(decPart, 10, 64)
		amountCents = ai*100 + ad
	} else {
		ai, _ := strconv.ParseInt(amount, 10, 64)
		amountCents = ai * 100
	}
	if amountCents <= 0 {
		return "", fmt.Errorf("invalid amount: %s", amount)
	}

	// 金额匹配窗口：仅匹配 5 分钟内创建的未支付订单
	since := time.Now().Add(-5 * time.Minute)

	order := &model.PayOrder{}
	err := s.Db().
		Where("amount_cents = ? AND status = 'pending' AND created_at >= ?", amountCents, since).
		Order("created_at DESC").
		First(order).Error
	if err != nil {
		return "", fmt.Errorf("no pending order matches amount %s", amount)
	}
	return order.OutTradeNo, nil
}

// CreateOrder 创建订阅订单
// channel: alipay / wechat, planKey: 1m / 3m / 6m / 12m
func (s *SubscribeService) CreateOrder(userID uint, channel, planKey string) (*model.PayOrder, error) {
	if channel != "wechat" && channel != "alipay" {
		return nil, fmt.Errorf("unsupported channel: %s", channel)
	}

	// 查时长选项
	opt := Config.Subscription.LookupPlan(planKey)
	if opt == nil {
		return nil, fmt.Errorf("invalid plan_key: %s", planKey)
	}
	priceCents := opt.PriceCents
	periodDays := opt.PeriodDays
	if priceCents <= 0 || periodDays <= 0 {
		return nil, fmt.Errorf("invalid plan config for key %s", planKey)
	}

	expireSec := Config.Payment.OrderExpireSec
	if expireSec <= 0 {
		expireSec = 600
	}

	order := &model.PayOrder{
		OutTradeNo:  s.generateOutTradeNo(),
		UserID:      userID,
		Plan:        Config.Subscription.Plan,
		PlanKey:     planKey,
		AmountCents: priceCents,
		Channel:     channel,
		Status:      "pending",
		PeriodDays:  periodDays,
	}

	if err := s.Db().Create(order).Error; err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	return order, nil
}

// NotifyConfig 通知签名参数
type NotifyConfig struct {
	SecretKey string
	PID       string // 商户 ID，自我实现时固定
}

// BuildNotifyParams 构建回调通知参数字符串（含 sign）
// 码支付标准格式：pid, trade_no, out_trade_no, type, name, money, trade_status, sign
func (s *SubscribeService) BuildNotifyParams(order *model.PayOrder) map[string]string {
	params := map[string]string{
		"pid":          "1000",
		"trade_no":     order.OutTradeNo,
		"out_trade_no": order.OutTradeNo,
		"type":         order.Channel,
		"name":         order.Plan + "订阅",
		"money":        fmt.Sprintf("%.2f", float64(order.AmountCents)/100),
		"trade_status": "TRADE_SUCCESS",
	}
	// 有 secret_key 才加签名
	sk, _ := s.PayConfig()
	if sk != "" {
		params["sign"] = payverify.Sign(params, sk)
	}
	return params
}

// HandleNotify 处理支付回调通知
// params: 回调参数（含 sign）
// 返回 true 表示处理成功，false 表示验签失败需忽略
func (s *SubscribeService) HandleNotify(params map[string]string) (bool, error) {
	secretKey, _ := s.PayConfig()
	if secretKey != "" {
		if !payverify.Verify(params, secretKey) {
			Logger.Warnf("codepay notify sign verify failed: %+v", params)
			return false, fmt.Errorf("sign verify failed")
		}
	} else {
		Logger.Warn("codepay secret_key not configured, skip sign verification")
	}

	// 取 out_trade_no
	outTradeNo := params["out_trade_no"]
	if outTradeNo == "" {
		return false, fmt.Errorf("out_trade_no empty")
	}

	// 只处理 TRADE_SUCCESS
	if params["trade_status"] != "TRADE_SUCCESS" {
		Logger.Infof("notify ignored: status=%s, out_trade_no=%s", params["trade_status"], outTradeNo)
		return true, nil
	}

	// 事务：幂等改单 → 生成邀请码 → 激活订阅
	now := time.Now()
	err := s.Db().Transaction(func(tx *gorm.DB) error {
		order := &model.PayOrder{}
		if err := tx.Where("out_trade_no = ?", outTradeNo).First(order).Error; err != nil {
			return fmt.Errorf("order not found: %s", outTradeNo)
		}

		// 幂等
		if order.Status == "paid" {
			Logger.Infof("notify idempotent: order %s already paid, skip", outTradeNo)
			return nil
		}
		if order.Status != "pending" {
			return fmt.Errorf("order %s status is %s, cannot paid", outTradeNo, order.Status)
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

		// 生成邀请码并激活
		periodDays := order.PeriodDays
		if periodDays <= 0 {
			periodDays = 30
		}
		plan := order.Plan
		if plan == "" {
			plan = "pro"
		}

		ics := &InviteCodeService{}
		ic, err := ics.GenerateWithDB(tx, plan, order.UserID, outTradeNo, periodDays)
		if err != nil {
			return fmt.Errorf("generate code: %w", err)
		}

		// 激活订阅（顺延）
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
		// 同步更新 expired_at，使会员过期时客户端也无法登录
		expiredAt := newExpire.Unix()
		if err := tx.Model(&model.User{}).Where("id = ?", order.UserID).
			Updates(map[string]interface{}{
				"subscription_plan":      plan,
				"subscription_expire_at": &newExpire,
				"expired_at":             expiredAt,
			}).Error; err != nil {
			return err
		}

		// 更新邀请码状态
		icTime := now
		if err := tx.Model(&model.InviteCode{}).Where("id = ?", ic.Id).
			Updates(map[string]interface{}{
				"status":    "used",
				"used_by":   order.UserID,
				"used_at":   &icTime,
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

// QueryOrder 查询订单（仅订单所属用户可见）
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

// ClaimCode 订单号认领邀请码（已支付但未收到码的兜底）
func (s *SubscribeService) ClaimCode(userID uint, outTradeNo string) (*model.InviteCode, error) {
	order := &model.PayOrder{}
	if err := s.Db().Where("out_trade_no = ? AND user_id = ?", outTradeNo, userID).First(order).Error; err != nil {
		return nil, fmt.Errorf("ORDER_NOT_FOUND")
	}
	if order.Status != "paid" {
		return nil, fmt.Errorf("ORDER_NOT_PAID")
	}

	ics := &InviteCodeService{}
	existing := ics.InfoByOrderID(outTradeNo)
	if existing != nil && existing.Id > 0 {
		return existing, nil
	}

	periodDays := order.PeriodDays
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
	_, err = ics.Activate(ic.Code, userID)
	if err != nil {
		return nil, fmt.Errorf("activate code: %w", err)
	}
	return ic, nil
}

// RedeemCode 用户兑换邀请码
func (s *SubscribeService) RedeemCode(userID uint, codeStr string) (*model.InviteCode, error) {
	ics := &InviteCodeService{}
	ic, err := ics.Activate(codeStr, userID)
	if err != nil {
		return nil, err
	}
	return ic, nil
}

// GetMine 获取当前用户订阅信息
func (s *SubscribeService) GetMine(userID uint) (*model.User, error) {
	user := &model.User{}
	if err := s.Db().Where("id = ?", userID).First(user).Error; err != nil {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}
