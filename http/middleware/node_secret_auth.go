package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// nodeSecretWarnOnce 保证"未配置节点密钥"的告警只打一次，避免每 15 秒刷一条。
var nodeSecretWarnOnce sync.Once

// NodeSecretAuth 节点专用密钥校验（请求头 X-Node-Secret）。
//
// 与 ProcessMonitorAuth 串联使用：后者确认"这是已知设备/已登录用户"，本中间件
// 确认"它知道节点专用密钥"。
//
// 为什么需要它：ProcessMonitorAuth 接受的 X-Rustdesk-Key 是 hbbs 公钥，会随
// 客户端下发，等于公开信息，任何人拿到都能伪造节点注册、把自己塞进中继列表。
//
// 密钥轮换：接受 node.secret（当前）或 node.secret_prev（上一把，仅在轮换后的
// 宽限期内）。这样管理员在后台换密钥时，还没轮询到新密钥的节点不会瞬间 401
// 掉线；节点随后通过 GET /api/node/secret 自动换到新密钥。
//
// 兼容策略：node.secret 未配置时**不阻断**（升级即断会停掉所有节点心跳），
// 只告警一次，提示管理员尽快在后台生成并配置。
func NodeSecretAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if service.AllService.NodeService.NodeSecret() == "" {
			nodeSecretWarnOnce.Do(func() {
				global.Logger.Warnf("[Node] node.secret 未配置：/api/node 写接口仅依赖 ProcessMonitorAuth" +
					"（rustdesk key 为公开信息），存在伪造节点风险；请在后台「集群节点 → 节点接入密钥」生成并配置。")
			})
			c.Next()
			return
		}
		ok, usingPrev := service.AllService.NodeService.NodeSecretValid(c.GetHeader("X-Node-Secret"))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid node secret"})
			return
		}
		if usingPrev {
			// 把"正在用旧密钥"标记透传给 handler（例如 /api/node/secret 可据此提示
			// 节点尽快切换），不打印日志避免心跳场景每 15 秒刷一条。
			c.Set("node_secret_using_prev", true)
		}
		c.Next()
	}
}
