package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

// Cors 跨域中间件。
//
// SECURITY（修复前）：旧实现把请求头的 Origin 原样反射到 Access-Control-Allow-Origin，
// 并始终携带 Access-Control-Allow-Credentials: true。这会让任意第三方站点在用户浏览器中
// 发起带凭证（api-token / Cookie）的跨域请求并读取响应，造成 CSRF / 敏感数据泄露。
//
// SECURITY（修复后）：仅当请求的 Origin 命中配置的允许源白名单（cors.allow-origins）时才反射，
// 且仅在命中时才允许携带凭证。白名单为空时完全不开启跨域（不设置 ACAO，浏览器会拦截跨域响应）。
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// 同源请求或非浏览器请求（无 Origin）无需跨域处理，直接放行。
		if origin == "" {
			c.Next()
			return
		}

		// 未命中白名单：不设置任何 CORS 响应头。
		// 浏览器因缺少 Access-Control-Allow-Origin 会拦截实际响应；
		// 预检 OPTIONS 也因无 ACAO 而被拒绝，从根源上关闭跨域。
		if !isOriginAllowed(origin) {
			c.Next()
			return
		}

		// 仅命中白名单时才反射具体源并允许凭证，避免任意站点读取带凭证的响应。
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "api-token,content-type,authorization")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")

		if c.Request.Method == http.MethodOptions {
			// 预检请求：命中白名单时直接返回 204，不进入后续路由。
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// isOriginAllowed 判断 origin 是否命中白名单（精确匹配，大小写不敏感以杜绝大小写绕过）。
func isOriginAllowed(origin string) bool {
	for _, allowed := range global.Config.Cors.AllowOrigins {
		if allowed == "" {
			continue
		}
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}
