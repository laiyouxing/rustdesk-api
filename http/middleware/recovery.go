package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

// Recovery 是一个 panic 恢复中间件，用于替代 gin.Recovery()。
//
// 安全要点：任何发生在 handler / 后续中间件中的 panic 都只会在服务端日志打印，
// 绝不会把 panic 内容（可能包含 SQL、堆栈、内部路径等敏感信息）回传给客户端，
// 以免泄露内部实现细节。客户端始终收到统一的通用 5xx 包络：
//
//	{"code":500,"message":"服务器内部错误"}
//
// SECURITY：默认 gin.Recovery() 仅返回纯文本 "500 Internal Server Error"，
// 既不统一也不存在泄露风险，但本中间件统一为 JSON 通用文案（HTTP 200 + 业务 code 500），
// 并可作为最外层（第一个）中间件注册，确保任何 panic 都被兜住。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				// 仅记录到服务端日志，不向客户端泄露任何细节。
				global.Logger.Error("panic recovered: " + fmt.Sprintf("%v", r))
				// 若响应头尚未写出，才返回统一 5xx 包络（HTTP 200 + code 500），避免 superfluous WriteHeader。
				if !c.Writer.Written() {
					c.Abort()
					c.JSON(http.StatusOK, gin.H{"code": 500, "message": "服务器内部错误"})
				}
			}
		}()
		c.Next()
	}
}

// AbortServerError 以统一通用文案中止请求并返回 500，避免向客户端泄露内部错误细节
// （如 SQL、堆栈信息）。供控制器在捕获到非预期的服务端错误时调用，
// 替代直接把 err.Error() 回显给客户端。
func AbortServerError(c *gin.Context) {
	if !c.Writer.Written() {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "服务器内部错误",
		})
	}
}
