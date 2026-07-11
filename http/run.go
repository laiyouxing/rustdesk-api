//go:build !windows

package http

import (
	"net/http"

	"github.com/fvbock/endless"
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

// Run 在类 Unix 平台启动 HTTP 服务。
//
// 使用 endless 以实现优雅重启（零停机升级，监听 SIGHUP/SIGUSR2 等信号），
// 并显式设置读/写/空闲超时以缓解慢速攻击；endless 的 Serve() 会使用内嵌的
// http.Server 的超时设置，因此此处设置的超时对实际请求生效。
// 若配置了 server.tls.enabled，则直接以 HTTPS 监听。
func Run(g *gin.Engine, addr string) {
	srv := endless.NewServer(addr, g)
	read, write, idle := serverTimeouts()
	srv.ReadTimeout = read
	srv.WriteTimeout = write
	srv.IdleTimeout = idle

	var err error
	if global.Config.Server.TLS.Enabled {
		err = srv.ListenAndServeTLS(global.Config.Server.TLS.CertFile, global.Config.Server.TLS.KeyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		global.Logger.Fatalf("server start failed: %v", err)
	}
}
