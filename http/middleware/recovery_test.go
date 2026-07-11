package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRecovery_ReturnsGeneric500OnPanic 验证 panic 被兜住时：
// 1) 返回 HTTP 200（业务 code 500，与全站 5xx 收口一致）；
// 2) 响应为统一通用文案 {"code":500,"message":"服务器内部错误"}；
// 3) panic 细节（可能含 SQL/堆栈）不会泄露到响应体。
func TestRecovery_ReturnsGeneric500OnPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery())
	r.GET("/panic", func(c *gin.Context) {
		panic("boom: secret sql=SELECT * FROM users WHERE password='x'")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (body=%s)", err, w.Body.String())
	}
	if body["code"] != float64(500) {
		t.Errorf("expected code 500, got %v", body["code"])
	}
	if body["message"] != "服务器内部错误" {
		t.Errorf("expected generic message, got %v", body["message"])
	}

	// 确保 panic 细节没有泄露到响应体
	if strings.Contains(w.Body.String(), "boom") || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "SELECT") {
		t.Errorf("panic details leaked to client: %s", w.Body.String())
	}
}

// TestRecovery_PassesThroughWhenNoPanic 验证无 panic 时正常透传。
func TestRecovery_PassesThroughWhenNoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery())
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
