package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
)

// tokenTag 返回 token 的短标签，仅用于把日志与同一枚 token 关联，不泄露可用凭据。
func tokenTag(v string) string {
	if v == "" {
		return "-"
	}
	if len(v) <= 8 {
		return v
	}
	return v[:8]
}

// logAuthReject 记录后台鉴权被拒的**精确原因**。
//
// 定位"登录成功后立刻掉线 / 被反复要求重新登录"这类问题时，必须先区分是
// no_cookie（浏览器没存/没带 Cookie）、token_not_found（记录被删）、
// token_expired（过期）还是 fingerprint_mismatch（User-Agent 变了），
// 否则只能靠猜。只记录 token/指纹的前 8 位与 UA 长度，不写入可用凭据。
func logAuthReject(c *gin.Context, reason, token string) {
	global.Logger.Warnf(
		"[AUTH] reject reason=%s path=%s token=%s fp=%s ua_len=%d ip=%s",
		reason,
		c.FullPath(),
		tokenTag(token),
		tokenTag(utils.Md5(c.GetHeader("User-Agent"))),
		len(c.GetHeader("User-Agent")),
		c.ClientIP(),
	)
}

// BackendUserAuth 后台权限验证中间件
func BackendUserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {

		// 从 HttpOnly Cookie 读取会话令牌（前端 JS 无法读取，XSS 不可窃）
		token, err := c.Cookie("access_token")
		if err != nil || token == "" {
			// 用 401 表示"会话无效/未登录"，与 AdminPrivilege 的 403(无权限) 区分开。
			// 前端只对 401 做登出+跳转；若两者都用 403，任何"权限不足"都会被前端
			// 误判成掉线并强制退出——带 MFA 的账户会陷入"登录成功→立刻退出→
			// 再次要求输入动态码"的循环。
			logAuthReject(c, "no_cookie", "")
			response.Fail(c, 401, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}
		// 计算来源指纹（仅 User-Agent），与签发时存储的指纹比对，防止 token 异地盗用。
		// 不含 IP：反代/双栈下 c.ClientIP() 会在同一会话内波动，导致误判。
		fingerprint := utils.Md5(c.GetHeader("User-Agent"))
		user, ut := service.AllService.UserService.InfoByAccessToken(token, fingerprint)
		if user.Id == 0 {
			// 精确区分失败原因，便于定位"登录后立刻掉线"这类问题。
			reason := "token_not_found"
			switch {
			case ut.Id == 0:
				reason = "token_not_found"
			case ut.ExpiredAt < time.Now().Unix():
				reason = "token_expired"
			case ut.Fingerprint != "" && fingerprint != ut.Fingerprint:
				reason = "fingerprint_mismatch"
			}
			logAuthReject(c, reason, token)
			// token 无效、过期或来源不匹配：清除可能残留的 Cookie 并拒绝
			secure := false
			if c.Request.TLS != nil {
				secure = true
			} else if hp := c.GetHeader("X-Forwarded-Proto"); strings.EqualFold(hp, "https") {
				secure = true
			}
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie("access_token", "", -1, "/", "", secure, true)
			// 同样用 401：token 失效属于"会话无效"，前端应登出重登，
			// 而不是当成"无权限"。
			response.Fail(c, 401, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}

		// 账户被禁用或已过期：会话同样无效，必须让前端登出，否则会一直 401 抖动。
		if !service.AllService.UserService.CheckUserEnable(user) || service.AllService.UserService.IsUserExpired(user) {
			response.Fail(c, 401, response.TranslateMsg(c, "NeedLogin"))
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
