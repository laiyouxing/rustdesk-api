//go:build windows

package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

// Run 在 Windows 平台启动 HTTP 服务。
//
// Windows 下不使用 endless（其信号处理依赖 Unix 信号），直接基于标准库 http.Server
// 启动，并显式设置读/写/空闲超时以缓解慢速攻击；
// 若配置了 server.tls.enabled，则直接以 HTTPS 监听。
func Run(g *gin.Engine, addr string) {
	read, write, idle := serverTimeouts()
	srv := &http.Server{
		Addr:         addr,
		Handler:      g,
		ReadTimeout:  read,
		WriteTimeout: write,
		IdleTimeout:  idle,
	}

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
