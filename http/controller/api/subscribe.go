package api

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	respApi "github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/lib/payverify"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// SubscribeController 订阅控制器
type SubscribeController struct{}

// NewSubscribeController 创建控制器
func NewSubscribeController() *SubscribeController {
	return &SubscribeController{}
}

// getQRURL 根据渠道返回收款二维码 URL
func (sc *SubscribeController) getQRURL(c *gin.Context, channel string) string {
	cc := global.Config.Payment.Cashier
	qrPath := cc.AlipayQR
	if channel == "wechat" {
		qrPath = cc.WechatQR
	}
	if qrPath == "" {
		return ""
	}

	// 已经是 URL，直接返回
	if len(qrPath) > 4 && qrPath[:4] == "http" {
		return qrPath
	}

	// 相对路径：返回完整 URL（基于请求的 host）
	base := filepath.Base(qrPath)
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/static/qr/%s", scheme, c.Request.Host, base)
}

// Plans 返回可选时长列表
func (sc *SubscribeController) Plans(c *gin.Context) {
	sc.SubscriptionPlans(c)
}

// SubscriptionPlans 返回可选时长列表
func (sc *SubscribeController) SubscriptionPlans(c *gin.Context) {
	// 不需要注入到 global.Config，直接从 service 读取
	subCfg := global.Config.Subscription
	type planItem struct {
		Key        string `json:"key"`
		Name       string `json:"name"`
		PriceCents int64  `json:"price_cents"`
		PeriodDays int    `json:"period_days"`
	}
	var items []planItem
	for _, p := range subCfg.Plans {
		items = append(items, planItem{
			Key:        p.Key,
			Name:       p.Name,
			PriceCents: p.PriceCents,
			PeriodDays: p.PeriodDays,
		})
	}
	response.Success(c, items)
}

// CreateOrder 创建订单
func (sc *SubscribeController) CreateOrder(c *gin.Context) {
	req := &api.CreateOrderReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 400, response.TranslateMsg(c, "ParamError"))
		return
	}

	user := service.AllService.UserService.CurUser(c)
	if user.Id == 0 {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}

	order, err := service.AllService.SubscribeService.CreateOrder(user.Id, req.Channel, req.PlanKey)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	expireSec := global.Config.Payment.OrderExpireSec
	if expireSec <= 0 {
		expireSec = 600
	}

	resp := &respApi.OrderResp{
		OutTradeNo:    order.OutTradeNo,
		AmountCents:   order.AmountCents,
		Plan:          order.Plan,
		PlanKey:       order.PlanKey,
		PeriodDays:    order.PeriodDays,
		ExpireSeconds: expireSec,
		Status:        order.Status,
		QRPayload:     sc.getQRURL(c, req.Channel),
		CodeIssued:    false,
	}
	response.Success(c, resp)
}

// WebhookReq 简单确认 webhook 请求
type WebhookReq struct {
	// Secret 预共享密钥（必须等于 config payment.secret_key）
	Secret string `json:"secret" binding:"required"`
	// OutTradeNo 可选：直接指定订单号
	OutTradeNo string `json:"out_trade_no"`
	// Amount 可选：不传 order_no 时传金额，系统自动匹配最近未支付的订单（元，如 "10.00"）
	Amount string `json:"amount"`
}

// Webhook SmsForwarder 等监控工具直接回调接口
// 两种调用方式：
//   1. 传 order_no → 直接确认该订单
//   2. 只传 amount → 按金额匹配最近 pending 订单
func (sc *SubscribeController) Webhook(c *gin.Context) {
	req := &WebhookReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.String(http.StatusOK, "fail")
		return
	}

	sk := global.Config.Payment.SecretKey
	if sk == "" || req.Secret != sk {
		global.Logger.Warnf("webhook secret mismatch: got %q", req.Secret)
		c.String(http.StatusOK, "fail")
		return
	}

	var outTradeNo string

	if req.OutTradeNo != "" {
		// 方式1: 直接按订单号确认
		outTradeNo = req.OutTradeNo
	} else if req.Amount != "" {
		// 方式2: 按金额匹配
		no, err := service.AllService.SubscribeService.MatchOrderByAmount(req.Amount)
		if err != nil {
			global.Logger.Warnf("webhook match amount %s failed: %v", req.Amount, err)
			c.String(http.StatusOK, "fail")
			return
		}
		outTradeNo = no
	} else {
		c.String(http.StatusOK, "fail")
		return
	}

	// 构造带签名的参数传给 HandleNotify
	params := map[string]string{
		"out_trade_no": outTradeNo,
		"trade_status": "TRADE_SUCCESS",
		"money":        "0",
	}
	params["sign"] = payverify.Sign(params, sk)

	ok, err := service.AllService.SubscribeService.HandleNotify(params)
	if err != nil {
		global.Logger.Warnf("webhook failed: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}
	if ok {
		c.String(http.StatusOK, "success")
	} else {
		c.String(http.StatusOK, "fail")
	}
}

// SmsForwarderReq SmsForwarder 原生 webhook 回调格式
// 模板: {"pid":"1001","aid":"46","uid":"{{UID}}","title":"{{TITLE}}","msg":"{{MSG}}","time":"{{RECEIVE_TIME}}","divice":"{{DEVICE_NAME}}"}
type SmsForwarderReq struct {
	PID    string `json:"pid"`
	AID    string `json:"aid"`
	UID    string `json:"uid"`
	Title  string `json:"title"`  // 发送者，如"支付宝"
	Msg    string `json:"msg"`    // 内容，如"支付宝到账10.00元"
	Time   string `json:"time"`
	Device string `json:"divice"`
}

// SmsWebhook 接收 SmsForwarder 原生格式回调，自动提取金额匹配订单
// 配置: URL = http://host/api/subscribe/sms-webhook?secret=你的key
func (sc *SubscribeController) SmsWebhook(c *gin.Context) {
	secret := c.Query("secret")
	sk := global.Config.Payment.SecretKey
	if sk == "" || secret != sk {
		global.Logger.Warnf("sms-webhook secret mismatch")
		c.String(http.StatusOK, "fail")
		return
	}

	req := &SmsForwarderReq{}
	if err := c.ShouldBindJSON(req); err != nil || req.Msg == "" {
		c.String(http.StatusOK, "fail")
		return
	}

	amount, err := service.AllService.SubscribeService.ExtractAmountFromSMS(req.Msg)
	if err != nil {
		global.Logger.Warnf("sms-webhook extract amount failed: msg=%q, err=%v", req.Msg, err)
		c.String(http.StatusOK, "fail")
		return
	}

	no, err := service.AllService.SubscribeService.MatchOrderByAmount(amount)
	if err != nil {
		global.Logger.Warnf("sms-webhook match failed: amount=%s, msg=%q, err=%v", amount, req.Msg, err)
		c.String(http.StatusOK, "fail")
		return
	}

	params := map[string]string{
		"out_trade_no": no,
		"trade_status": "TRADE_SUCCESS",
		"money":        amount,
	}
	params["sign"] = payverify.Sign(params, sk)

	ok, err := service.AllService.SubscribeService.HandleNotify(params)
	if err != nil {
		global.Logger.Warnf("sms-webhook notify failed: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}
	if ok {
		global.Logger.Infof("sms-webhook success: order=%s, amount=%s, from=%s", no, amount, req.Title)
		c.String(http.StatusOK, "success")
	} else {
		c.String(http.StatusOK, "fail")
	}
}

// Notify 支付回调通知（公开接口，不鉴权）
func (sc *SubscribeController) Notify(c *gin.Context) {
	params := make(map[string]string)
	c.Request.ParseForm()
	for k, v := range c.Request.Form {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	if len(params) == 0 {
		body := make(map[string]interface{})
		if err := c.ShouldBindJSON(&body); err == nil {
			for k, v := range body {
				params[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	ok, err := service.AllService.SubscribeService.HandleNotify(params)
	if err != nil {
		global.Logger.Warnf("notify failed: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}
	if ok {
		c.String(http.StatusOK, "success")
	} else {
		c.String(http.StatusOK, "fail")
	}
}

// QueryOrder 查询订单状态
func (sc *SubscribeController) QueryOrder(c *gin.Context) {
	outTradeNo := c.Param("out_trade_no")
	if outTradeNo == "" {
		response.Fail(c, 400, "missing out_trade_no")
		return
	}
	user := service.AllService.UserService.CurUser(c)
	if user.Id == 0 {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}

	order, err := service.AllService.SubscribeService.QueryOrder(outTradeNo, user.Id)
	if err != nil {
		response.Fail(c, 4101, response.TranslateMsg(c, "OrderNotFound"))
		return
	}

	ics := &service.InviteCodeService{}
	ic := ics.InfoByOrderID(outTradeNo)
	codeIssued := ic != nil && ic.Id > 0

	resp := &respApi.OrderResp{
		OutTradeNo:  order.OutTradeNo,
		Status:      order.Status,
		Plan:        order.Plan,
		PlanKey:     order.PlanKey,
		PeriodDays:  order.PeriodDays,
		AmountCents: order.AmountCents,
		PaidAt:      order.PaidAt,
		CodeIssued:  codeIssued,
	}
	if codeIssued {
		resp.InviteCode = ic.Code
		resp.ExpireAt = &ic.ExpireAt
	}
	response.Success(c, resp)
}

// Claim 订单号认领邀请码（兜底）
func (sc *SubscribeController) Claim(c *gin.Context) {
	req := &api.ClaimReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 400, response.TranslateMsg(c, "ParamError"))
		return
	}
	user := service.AllService.UserService.CurUser(c)
	if user.Id == 0 {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}

	ic, err := service.AllService.SubscribeService.ClaimCode(user.Id, req.OutTradeNo)
	if err != nil {
		errMsg := err.Error()
		switch {
		case contains(errMsg, "ORDER_NOT_FOUND"):
			response.Fail(c, 4101, response.TranslateMsg(c, "OrderNotFound"))
		case contains(errMsg, "ORDER_NOT_PAID"):
			response.Fail(c, 4102, response.TranslateMsg(c, "OrderNotPaid"))
		default:
			response.Fail(c, 500, errMsg)
		}
		return
	}

	resp := &respApi.ClaimResp{
		Code:     ic.Code,
		ExpireAt: ic.ExpireAt,
		Plan:     ic.Plan,
	}
	response.Success(c, resp)
}

// Redeem 兑换邀请码
func (sc *SubscribeController) Redeem(c *gin.Context) {
	req := &api.RedeemReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 400, response.TranslateMsg(c, "ParamError"))
		return
	}
	user := service.AllService.UserService.CurUser(c)
	if user.Id == 0 {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}

	ic, err := service.AllService.SubscribeService.RedeemCode(user.Id, req.Code)
	if err != nil {
		errMsg := err.Error()
		switch {
		case contains(errMsg, "code not found"):
			response.Fail(c, 4201, response.TranslateMsg(c, "CodeNotFound"))
		case contains(errMsg, "code already used"):
			response.Fail(c, 4202, response.TranslateMsg(c, "CodeUsed"))
		case contains(errMsg, "code revoked"):
			response.Fail(c, 4203, response.TranslateMsg(c, "CodeRevoked"))
		case contains(errMsg, "code expired"):
			response.Fail(c, 4204, response.TranslateMsg(c, "CodeExpired"))
		default:
			response.Fail(c, 500, errMsg)
		}
		return
	}

	user = service.AllService.UserService.InfoById(user.Id)
	resp := &respApi.RedeemResp{
		Plan:                 ic.Plan,
		ExpireAt:             ic.ExpireAt,
		SubscriptionExpireAt: user.SubscriptionExpireAt,
	}
	response.Success(c, resp)
}

// Mine 获取当前用户的订阅信息
func (sc *SubscribeController) Mine(c *gin.Context) {
	user := service.AllService.UserService.CurUser(c)
	if user.Id == 0 {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}

	u, err := service.AllService.SubscribeService.GetMine(user.Id)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	status := u.SubscriptionStatus()
	daysLeft := u.SubscriptionDaysLeft()
	isExpiringSoon := false
	if status == "active" && daysLeft > 0 && daysLeft <= 7 {
		isExpiringSoon = true
	}

	resp := &respApi.MineResp{
		Plan:                 u.SubscriptionPlan,
		SubscriptionExpireAt: u.SubscriptionExpireAt,
		Status:               status,
		DaysLeft:             daysLeft,
		IsExpiringSoon:       isExpiringSoon,
	}
	response.Success(c, resp)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}
