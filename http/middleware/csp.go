package middleware

import (
	"github.com/gin-gonic/gin"
)

// CSP 设置内容安全策略（Content-Security-Policy），限制脚本/样式仅来自同源（'self'），
// 从根本上阻止外部（CDN / 攻击者域名）JavaScript 的注入与执行，缓解 XSS 与供应链攻击。
//
// 策略说明：
//   - script-src 'self' 'unsafe-inline'：仅允许同源脚本；'unsafe-inline' 用于兼容历史服务端
//     模板中的内联脚本。主管理后台 SPA（web-admin 构建产物）无任何内联脚本，等效于只加载
//     同源打包后的 JS，外部脚本一律被浏览器拒绝。
//   - style-src 'self' 'unsafe-inline'：仅允许同源样式，外部 CSS（如模板里曾引用的 CDN 字体）
//     将被拦截。
//   - img-src/font-src 允许 data:（SPA 内联资源）；object-src 'none' 禁用插件；
//     base-uri 'self' 防止 <base> 注入；frame-ancestors 'self' 防点击劫持。
//   - 未限制 connect-src/img 等，避免影响 web client 连接外部 relay 等正常功能。
func CSP() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy",
			"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self' data:; "+
				"object-src 'none'; "+
				"base-uri 'self'; "+
				"frame-ancestors 'self'")
		c.Next()
	}
}
