package http

import (
	"time"

	"github.com/lejianwen/rustdesk-api/v2/global"
)

// orDefault 当 v<=0 时返回安全默认值 def，否则返回 v。
func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// serverTimeouts 根据配置构造读/写/空闲超时，缺失或非法(<=0)时回退到安全默认值
//（15/30/120 秒），以缓解慢速攻击与连接耗尽。
func serverTimeouts() (read, write, idle time.Duration) {
	read = time.Duration(orDefault(global.Config.Server.ReadTimeout, 15)) * time.Second
	write = time.Duration(orDefault(global.Config.Server.WriteTimeout, 30)) * time.Second
	idle = time.Duration(orDefault(global.Config.Server.IdleTimeout, 120)) * time.Second
	return
}
