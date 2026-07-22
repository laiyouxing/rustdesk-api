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
