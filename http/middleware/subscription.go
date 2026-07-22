package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// SubscriptionGuard 订阅校验中间件
// 挂在用户 API 分组上，校验当前用户的订阅是否有效。
// 白名单路径跳过校验（详见 whitelist）。
//
// 中间件链顺序：CORS → Recovery → JwtAuth → SubscriptionGuard
// JwtAuth 已在前面将 curUser 注入 context。
type SubscriptionGuard struct {
	// whitelist 放行路径前缀列表（不含 /api/v1 前缀）
	// 公开路径（无需 JWT）：login, register, notify, health, static
	// 需 JWT 但订阅豁免：create-order, 订单查询, claim, redeem, mine
	whitelist []string
}

// NewSubscriptionGuard 创建订阅守卫
func NewSubscriptionGuard() *SubscriptionGuard {
	return &SubscriptionGuard{
		whitelist: []string{
			// 认证但订阅豁免（过期用户也可访问）
			"subscribe/create-order",
			"subscribe/order/",
			"subscribe/claim",
			"subscribe/redeem",
			"subscribe/mine",
		},
	}
}

// Handle 返回 gin.HandlerFunc
func (g *SubscriptionGuard) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 判断当前路径是否命中白名单
		path := c.Request.URL.Path
		// 去掉 /api/v1/ 前缀
		relativePath := strings.TrimPrefix(path, "/api/v1/")

		if g.isWhitelisted(relativePath) {
			c.Next()
			return
		}

		// 从 context 取当前用户（由 JwtAuth 注入）
		userInterface, exists := c.Get("curUser")
		if !exists {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}
		user, ok := userInterface.(*model.User)
		if !ok {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}

		// 检查订阅状态
		if user.SubscriptionExpireAt == nil || user.SubscriptionExpireAt.Before(time.Now()) {
			// 重新查询以获取最新数据（避免缓存）
			freshUser := service.AllService.UserService.InfoById(user.Id)
			if freshUser.Id == 0 || freshUser.SubscriptionExpireAt == nil || freshUser.SubscriptionExpireAt.Before(time.Now()) {
				response.Fail(c, 4001, response.TranslateMsg(c, "SubscriptionExpired"))
				c.Abort()
				return
			}
			// 更新 context 中的用户
			c.Set("curUser", freshUser)
			c.Next()
			return
		}

		c.Next()
	}
}

// isWhitelisted 检查路径是否在白名单中
func (g *SubscriptionGuard) isWhitelisted(path string) bool {
	for _, wl := range g.whitelist {
		if strings.HasPrefix(path, wl) {
			return true
		}
	}
	return false
}
