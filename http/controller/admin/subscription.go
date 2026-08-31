package admin

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// SubscriptionCtl 后台订阅管理
type SubscriptionCtl struct{}

// NewSubscriptionCtl 创建控制器
func NewSubscriptionCtl() *SubscriptionCtl {
	return &SubscriptionCtl{}
}

// List 订阅用户列表
func (sc *SubscriptionCtl) List(c *gin.Context) {
	status := c.Query("status")       // active / expired / none
	keyword := c.Query("keyword")      // 按用户名或ID搜索
	pageStr := c.Query("page")
	pageSizeStr := c.Query("size")

	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 {
		pageSize = 20
	}

	db := global.DB.Model(&model.User{})

	// 订阅状态筛选
	now := time.Now()
	switch status {
	case "active":
		db = db.Where("subscription_expire_at IS NOT NULL AND subscription_expire_at > ? AND subscription_expire_at < ?", now, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	case "expired":
		db = db.Where("subscription_expire_at IS NOT NULL AND subscription_expire_at <= ?", now)
	case "none":
		db = db.Where("subscription_expire_at IS NULL")
	case "permanent":
		db = db.Where("subscription_expire_at IS NOT NULL AND subscription_expire_at >= ?", time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	}

	// 关键词搜索
	if keyword != "" {
		db = db.Where("id = ? OR username LIKE ?", parseUint(keyword), "%"+keyword+"%")
	}

	var total int64
	db.Count(&total)

	var users []model.User
	db.Order("id ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&users)

	type subItem struct {
		ID                   uint       `json:"id"`
		Username             string     `json:"username"`
		SubscriptionPlan     string     `json:"subscription_plan"`
		SubscriptionExpireAt *time.Time `json:"subscription_expire_at"`
		Status               string     `json:"status"`
		DaysLeft             int        `json:"days_left"`
	}

	items := make([]subItem, 0, len(users))
	for _, u := range users {
		items = append(items, subItem{
			ID:                   u.Id,
			Username:             u.Username,
			SubscriptionPlan:     u.SubscriptionPlan,
			SubscriptionExpireAt: u.SubscriptionExpireAt,
			Status:               u.SubscriptionStatus(),
			DaysLeft:             u.SubscriptionDaysLeft(),
		})
	}

	response.Success(c, map[string]interface{}{
		"list":     items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// ExtendReq 延长会员请求
type ExtendReq struct {
	UserID  uint   `json:"user_id" binding:"required"`
	Plan    string `json:"plan"`     // 套餐标识，缺省 "pro"
	PlanKey string `json:"plan_key" binding:"required"` // 时长 key：1m / 3m / 6m / 12m / forever
}

// TerminateReq 终止会员请求
type TerminateReq struct {
	UserID uint `json:"user_id" binding:"required"`
}

// recordOpLog 记录账户操作审计日志
func recordOpLog(c *gin.Context, target *model.User, action, detail string) {
	cur := service.AllService.UserService.CurUser(c)
	opId := uint(0)
	opName := ""
	if cur != nil {
		opId = cur.Id
		opName = cur.Username
	}
	_ = service.AllService.AccountOpLogService.Log(opId, opName, target.Id, target.Username, action, detail)
}

// Extend 延长用户会员（按月付费）
func (sc *SubscriptionCtl) Extend(c *gin.Context) {
	req := &ExtendReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 400, "param error")
		return
	}

	if req.Plan == "" {
		req.Plan = "pro"
	}

	user := service.AllService.UserService.InfoById(req.UserID)
	if user.Id == 0 {
		response.Fail(c, 404, "user not found")
		return
	}

	now := time.Now()
	var newExpire time.Time
	var detail string
	// 无订阅或已过期视为"新建会员"，有有效订阅视为"延长"
	action := "extend"
	if user.SubscriptionExpireAt == nil || user.SubscriptionExpireAt.Before(now) {
		action = "create"
	}

	// 永久套餐特殊处理：设为 9999 年
	if req.PlanKey == "forever" {
		newExpire = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		// 永久会员：expired_at 设为 0（永不过期）
		if err := global.DB.Model(&model.User{}).Where("id = ?", req.UserID).
			Updates(map[string]interface{}{
				"subscription_plan":      req.Plan + "-forever",
				"subscription_expire_at": &newExpire,
				"expired_at":             0,
			}).Error; err != nil {
			response.Fail(c, 500, err.Error())
			return
		}
		detail = "设为永久会员"
		recordOpLog(c, user, action, detail)
		response.Success(c, map[string]interface{}{
			"user_id":                req.UserID,
			"plan":                   req.Plan + "-forever",
			"subscription_expire_at": &newExpire,
		})
		return
	}

	// 查配置获取 period_days
	opt := global.Config.Subscription.LookupPlan(req.PlanKey)
	if opt == nil || opt.PeriodDays <= 0 {
		response.Fail(c, 400, "invalid plan_key")
		return
	}

	periodDuration := time.Duration(opt.PeriodDays*24) * time.Hour
	if user.SubscriptionExpireAt == nil || user.SubscriptionExpireAt.Before(now) {
		newExpire = now.Add(periodDuration)
	} else {
		newExpire = user.SubscriptionExpireAt.Add(periodDuration)
	}

	// 只更新订阅时长；expired_at 保持不变（账户级过期由管理员管理），
	// 保证订阅到期后用户仍可登录后台续费。
	if err := global.DB.Model(&model.User{}).Where("id = ?", req.UserID).
		Updates(map[string]interface{}{
			"subscription_plan":      req.Plan,
			"subscription_expire_at": &newExpire,
		}).Error; err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	detail = fmt.Sprintf("%s %d 天（%s），到期时间 %s",
		map[bool]string{true: "开通", false: "延长"}[action == "create"],
		opt.PeriodDays, opt.Name, newExpire.Format("2006-01-02 15:04"))
	recordOpLog(c, user, action, detail)

	response.Success(c, map[string]interface{}{
		"user_id":                req.UserID,
		"plan":                   req.Plan,
		"subscription_expire_at": &newExpire,
	})
}

// Terminate 终止用户会员（立即过期）
func (sc *SubscriptionCtl) Terminate(c *gin.Context) {
	req := &TerminateReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 400, "param error")
		return
	}

	user := service.AllService.UserService.InfoById(req.UserID)
	if user.Id == 0 {
		response.Fail(c, 404, "user not found")
		return
	}

	// 将订阅过期时间设为过去时间（立即终止）
	expired := time.Now().Add(-time.Second)
	if err := global.DB.Model(&model.User{}).Where("id = ?", req.UserID).
		Updates(map[string]interface{}{
			"subscription_expire_at": &expired,
		}).Error; err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	recordOpLog(c, user, "terminate", "终止会员")

	response.Success(c, map[string]interface{}{
		"user_id":                req.UserID,
		"subscription_expire_at": &expired,
	})
}

func parseUint(s string) uint {
	n, _ := strconv.ParseUint(s, 10, 64)
	return uint(n)
}
