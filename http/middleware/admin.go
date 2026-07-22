package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
)

// BackendUserAuth 后台权限验证中间件
func BackendUserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {

		// 从 HttpOnly Cookie 读取会话令牌（前端 JS 无法读取，XSS 不可窃）
		token, err := c.Cookie("access_token")
		if err != nil || token == "" {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}
		// 计算来源指纹（仅 User-Agent），与签发时存储的指纹比对，防止 token 异地盗用。
		// 不含 IP：反代/双栈下 c.ClientIP() 会在同一会话内波动，导致误判。
		fingerprint := utils.Md5(c.GetHeader("User-Agent"))
		user, _ := service.AllService.UserService.InfoByAccessToken(token, fingerprint)
		if user.Id == 0 {
			// token 无效、过期或来源不匹配：清除可能残留的 Cookie 并拒绝
			secure := false
			if c.Request.TLS != nil {
				secure = true
			} else if hp := c.GetHeader("X-Forwarded-Proto"); strings.EqualFold(hp, "https") {
				secure = true
			}
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie("access_token", "", -1, "/", "", secure, true)
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}

		if !service.AllService.UserService.CheckUserEnable(user) || service.AllService.UserService.IsUserExpired(user) {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		c.Set("curUser", user)
		c.Set("token", token)
		// 注意：web 后台不启用自动续期，token 到期（默认 2h）需重新登录；
		// 配合来源指纹绑定，即使 token 泄露也只能在原环境使用且短期失效。

		c.Next()
	}
}
