package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

func RustAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		//获取HTTP_AUTHORIZATION
		token := c.GetHeader("Authorization")
		if token == "" || len(token) <= 7 {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}
		//提取token，格式是Bearer {token}
		token = token[7:]

		// 优先 JWT 验证（若配置了 JWT key）
		// JWT 验证通过后直接拿到 uid，可跳过数据库 token 查找
		if len(global.Jwt.Key) > 0 {
			uid, err := service.AllService.UserService.VerifyJWT(token)
			if err == nil && uid > 0 {
				user := service.AllService.UserService.InfoById(uid)
				if user.Id > 0 && service.AllService.UserService.CheckUserEnable(user) {
					c.Set("curUser", user)
					c.Set("token", token)
					c.Next()
					return
				}
			}
			// JWT 验证失败降级到数据库 token 查找（兼容老客户端）
		}

		user, ut := service.AllService.UserService.InfoByAccessToken(token)
		if user.Id == 0 {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}
		if !service.AllService.UserService.CheckUserEnable(user) {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		c.Set("curUser", user)
		c.Set("token", token)

		service.AllService.UserService.AutoRefreshAccessToken(ut)

		c.Next()
	}
}
