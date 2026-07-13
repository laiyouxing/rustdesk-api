package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// ProcessMonitorAuth 进程/端口监控上报与配置下发接口的鉴权中间件。
//
// 设计目标：允许未登录 api-server 账号的设备上报自身监控状态，
// 同时具备设备身份校验（依赖客户端与服务器共享的 rustdesk key）。
//  1. 携带有效 access_token / JWT 的已登录客户端：照常验证通过（兼容现状）。
//  2. 未登录设备：若请求携带与服务器配置一致的 rustdesk key（X-Rustdesk-Key 头），
//     视为可信设备，放行。
//  3. 服务器未配置 rustdesk key 时：兼容放行（保持旧行为）。
//  4. 既无有效 token 也无正确 key：拒绝（401）。
//
// 说明：rustdesk key 为 hbbs 服务器公钥（公开信息），该校验用于确认上报方
// 确实配置了本服务器的公钥，属“软”身份校验，安全性仍依赖私有/内网部署隔离。
func ProcessMonitorAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) 已登录：优先 JWT，再降级 DB token
		if auth := c.GetHeader("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimSpace(auth[7:])
			if len(token) > 0 {
				if len(global.Jwt.Key) > 0 {
					if uid, err := service.AllService.UserService.VerifyJWT(token); err == nil && uid > 0 {
						if user := service.AllService.UserService.InfoById(uid); user.Id > 0 && service.AllService.UserService.CheckUserEnable(user) {
							c.Set("curUser", user)
							c.Set("token", token)
							c.Next()
							return
						}
					}
				}
				if user, _ := service.AllService.UserService.InfoByAccessToken(token, ""); user.Id > 0 && service.AllService.UserService.CheckUserEnable(user) {
					c.Set("curUser", user)
					c.Set("token", token)
					c.Next()
					return
				}
			}
		}

		// 2) 未登录设备：用共享 rustdesk key 校验设备身份
		serverKey := normalizeRustdeskKey(global.Config.Rustdesk.Key)
		if serverKey != "" {
			clientKey := normalizeRustdeskKey(c.GetHeader("X-Rustdesk-Key"))
			if clientKey != "" && clientKey == serverKey {
				c.Next()
				return
			}
			// 设备 key 不匹配：拒绝
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid rustdesk key"})
			return
		}

		// 3) 服务器未配置 key：兼容放行（保持旧行为）
		c.Next()
	}
}

// normalizeRustdeskKey 去掉所有空白字符（含换行、空格），便于比对 PEM / 裸 base64 等不同书写格式。
func normalizeRustdeskKey(k string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(k)), "")
}
