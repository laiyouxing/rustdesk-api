package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	respApi "github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// SubscribeController 订阅控制器
type SubscribeController struct {
}

// NewSubscribeController 创建控制器
func NewSubscribeController() *SubscribeController {
	return &SubscribeController{}
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

	order, err := service.AllService.SubscribeService.CreateOrder(user.Id, req.Channel, req.Plan)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	// 默认 600 秒有效期
	expireSeconds := 600
	resp := &respApi.OrderResp{
		OutTradeNo:    order.OutTradeNo,
		CashierURL:    order.CashierURL,
		QRPayload:     order.QRPayload,
		AmountCents:   order.AmountCents,
		Plan:          order.Plan,
		ExpireSeconds: expireSeconds,
		Status:        order.Status,
		CodeIssued:    false,
	}
	response.Success(c, resp)
}

// Notify 支付平台异步回调（公开接口，不鉴权）
func (sc *SubscribeController) Notify(c *gin.Context) {
	// 收集回调参数（支持 form 和 JSON）
	params := make(map[string]string)
	c.Request.ParseForm()
	for k, v := range c.Request.Form {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	// 如果是 JSON body
	if len(params) == 0 {
		body := make(map[string]interface{})
		if err := c.ShouldBindJSON(&body); err == nil {
			for k, v := range body {
				params[k] = toString(v)
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

	// 查询是否已生成邀请码
	ics := &service.InviteCodeService{}
	ic := ics.InfoByOrderID(outTradeNo)
	codeIssued := ic != nil && ic.Id > 0

	resp := &respApi.OrderResp{
		OutTradeNo:  order.OutTradeNo,
		Status:      order.Status,
		Plan:        order.Plan,
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
		case contains(errMsg, "CODE_NOT_FOUND"):
			response.Fail(c, 4201, response.TranslateMsg(c, "CodeNotFound"))
		case contains(errMsg, "CODE_USED"):
			response.Fail(c, 4202, response.TranslateMsg(c, "CodeUsed"))
		case contains(errMsg, "CODE_REVOKED"):
			response.Fail(c, 4203, response.TranslateMsg(c, "CodeRevoked"))
		case contains(errMsg, "CODE_EXPIRED"):
			response.Fail(c, 4204, response.TranslateMsg(c, "CodeExpired"))
		default:
			response.Fail(c, 500, errMsg)
		}
		return
	}

	// 重新查询用户以获取最新的订阅过期时间
	user = service.AllService.UserService.InfoById(user.Id)
	resp := &respApi.RedeemResp{
		Plan:                  ic.Plan,
		ExpireAt:              ic.ExpireAt,
		SubscriptionExpireAt:  user.SubscriptionExpireAt,
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

// toString 将 interface{} 转为 string
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}
