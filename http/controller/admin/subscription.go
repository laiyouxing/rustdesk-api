package admin

import (
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
		db = db.Where("subscription_expire_at IS NOT NULL AND subscription_expire_at > ?", now)
	case "expired":
		db = db.Where("subscription_expire_at IS NOT NULL AND subscription_expire_at <= ?", now)
	case "none":
		db = db.Where("subscription_expire_at IS NULL")
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

// ExtendReq 延长订阅请求
type ExtendReq struct {
	UserID     uint   `json:"user_id" binding:"required"`
	Plan       string `json:"plan"`        // 套餐标识，缺省 "pro"
	PeriodDays int    `json:"period_days"` // 延长天数
}

// Extend 延长用户订阅
func (sc *SubscriptionCtl) Extend(c *gin.Context) {
	req := &ExtendReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 400, "param error")
		return
	}

	if req.PeriodDays <= 0 {
		req.PeriodDays = 30
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
	periodDuration := time.Duration(req.PeriodDays*24) * time.Hour
	var newExpire time.Time
	if user.SubscriptionExpireAt == nil || user.SubscriptionExpireAt.Before(now) {
		newExpire = now.Add(periodDuration)
	} else {
		newExpire = user.SubscriptionExpireAt.Add(periodDuration)
	}

	if err := global.DB.Model(&model.User{}).Where("id = ?", req.UserID).
		Updates(map[string]interface{}{
			"subscription_plan":      req.Plan,
			"subscription_expire_at": &newExpire,
		}).Error; err != nil {
		response.Fail(c, 500, err.Error())
		return
	}

	response.Success(c, map[string]interface{}{
		"user_id":                req.UserID,
		"plan":                   req.Plan,
		"subscription_expire_at": &newExpire,
	})
}

func parseUint(s string) uint {
	n, _ := strconv.ParseUint(s, 10, 64)
	return uint(n)
}
