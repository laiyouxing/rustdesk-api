package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

// setupCorsTestRouter 构造一个挂载了 Cors 中间件的 gin 引擎，并注册一个返回 200 的测试 handler。
func setupCorsTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Cors())
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	return r
}

// withAllowOrigins 在测试中设置全局 CORS 白名单，避免零值导致中间件 panic / 行为不确定。
func withAllowOrigins(origins ...string) {
	global.Config = config.Config{
		Cors: config.Cors{AllowOrigins: origins},
	}
}

// TestCors_EmptyWhitelist_BlocksAnyOrigin
// 白名单为空时，不应反射任意 Origin，也不应携带凭据头；同源/跨域请求仍应正常进入 handler。
func TestCors_EmptyWhitelist_BlocksAnyOrigin(t *testing.T) {
	withAllowOrigins() // 空白名单
	r := setupCorsTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 handler 正常返回 200，实际 %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("白名单为空时不应设置 ACAO，实际 %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("白名单为空时不应设置凭据头，实际 %q", got)
	}
}

// TestCors_AllowedOrigin_ReflectsAndAllowsCredentials
// 命中白名单的 Origin 应被原样反射，且必须带 Access-Control-Allow-Credentials: true。
func TestCors_AllowedOrigin_ReflectsAndAllowsCredentials(t *testing.T) {
	withAllowOrigins("https://a.example.com")
	r := setupCorsTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://a.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 handler 正常返回 200，实际 %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://a.example.com" {
		t.Errorf("期望 ACAO 反射为 https://a.example.com，实际 %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("期望凭据头为 true，实际 %q", got)
	}
}

// TestCors_DisallowedOrigin_NoCorsHeaders
// 未命中白名单的 Origin 不应反射，也不应允许凭据。
func TestCors_DisallowedOrigin_NoCorsHeaders(t *testing.T) {
	withAllowOrigins("https://a.example.com")
	r := setupCorsTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://b.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 handler 正常返回 200，实际 %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("未命中白名单不应设置 ACAO，实际 %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("未命中白名单不应设置凭据头，实际 %q", got)
	}
}

// TestCors_Preflight_AllowedOrigin_Returns204
// 命中白名单的 OPTIONS 预检应直接返回 204 且带 ACAO。
func TestCors_Preflight_AllowedOrigin_Returns204(t *testing.T) {
	withAllowOrigins("https://a.example.com")
	r := setupCorsTestRouter()

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://a.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("预检命中白名单应返回 204，实际 %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://a.example.com" {
		t.Errorf("预检响应应带 ACAO，实际 %q", got)
	}
}

// TestCors_Preflight_DisallowedOrigin_Not204
// 未命中白名单的 OPTIONS 预检不应返回 204，也不应设置 ACAO（从源头拒绝跨域）。
func TestCors_Preflight_DisallowedOrigin_Not204(t *testing.T) {
	withAllowOrigins("https://a.example.com")
	r := setupCorsTestRouter()

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://b.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNoContent {
		t.Fatalf("预检未命中白名单不应返回 204")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("预检未命中白名单不应设置 ACAO，实际 %q", got)
	}
}

// TestCors_NoOriginHeader_ReachesHandler
// 无 Origin 头的同源请求应正常进入 handler（200），且不应设置多余 CORS 头。
func TestCors_NoOriginHeader_ReachesHandler(t *testing.T) {
	withAllowOrigins("https://a.example.com")
	r := setupCorsTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil) // 无 Origin 头
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望同源请求返回 200，实际 %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("无 Origin 不应设置 ACAO，实际 %q", got)
	}
}

// TestCors_CaseInsensitiveMatch
// 白名单按大小写不敏感匹配，确认行为稳定（防止大小写绕过）。
func TestCors_CaseInsensitiveMatch(t *testing.T) {
	withAllowOrigins("https://a.example.com")
	r := setupCorsTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "HTTPS://A.EXAMPLE.COM")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 handler 正常返回 200，实际 %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "HTTPS://A.EXAMPLE.COM" {
		t.Errorf("大小写不敏感匹配后 ACAO 应反射原始 Origin，实际 %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("期望凭据头为 true，实际 %q", got)
	}
}
