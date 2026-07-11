package response

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/sirupsen/logrus"
)

// newTestContext 构造一个 gin 测试上下文，并把全局 Logger 重定向到可读取的 buffer，
// 以便：1) 避免 Fail 内部 global.Logger.Error 因 Logger 为 nil 而 panic；
// 2) 验证 >=500 时调用方传入的原始 message 确实被记录到服务端日志（而非回显给客户端）。
func newTestContext() (*gin.Context, *httptest.ResponseRecorder, *bytes.Buffer) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	// gin v1.9.0 的 CreateTestContext 返回 (*Context, *Engine)；第二个返回值在此测试用不到。
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	var logBuf bytes.Buffer
	global.Logger = logrus.New()
	global.Logger.SetOutput(&logBuf)
	global.Logger.SetLevel(logrus.InfoLevel)
	return c, w, &logBuf
}

// assertGeneric500Body 断言响应体为统一通用文案且不含任何敏感细节泄露。
func assertGeneric500Body(t *testing.T, w *httptest.ResponseRecorder, logBuf *bytes.Buffer, originalMessage string) {
	t.Helper()

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v (body=%s)", err, w.Body.String())
	}
	if body["code"] != float64(500) {
		t.Errorf("期望 code=500, 实际 %v", body["code"])
	}
	if body["message"] != "服务器内部错误" {
		t.Errorf("期望 message=服务器内部错误, 实际 %v", body["message"])
	}
	// data 字段应保持原 JSON 形状（即使为 nil，也应序列化存在）。
	if _, ok := body["data"]; !ok {
		t.Errorf("响应缺少 data 字段, 形状不符预期 {code,message,data}: %s", w.Body.String())
	}

	raw := w.Body.String()
	for _, leak := range []string{"Error 1146", "tb_x", "rustdesk_api", "数据库炸了"} {
		if strings.Contains(raw, leak) {
			t.Errorf("响应体泄露了敏感细节 %q: %s", leak, raw)
		}
	}

	// 原始 message 应仅记录在服务端日志，不应出现在客户端响应体。
	if logBuf != nil && originalMessage != "" {
		if !strings.Contains(logBuf.String(), originalMessage) {
			t.Errorf("原始 message 未被记录到服务端日志: log=%s", logBuf.String())
		}
		if strings.Contains(raw, originalMessage) {
			t.Errorf("原始 message 不应回显给客户端: %s", raw)
		}
	}
}

// TestFail_ServerError_500_SuppressesDetailAndReturnsGenericMessage
// 任务项 2：code=500 时必须丢弃调用方传入的细节，仅返回 "服务器内部错误"，
// 且响应体不得包含 SQL / 表名 / 库名 / 业务文案等任何敏感信息。
func TestFail_ServerError_500_SuppressesDetailAndReturnsGenericMessage(t *testing.T) {
	c, w, logBuf := newTestContext()

	detail := "数据库炸了：Error 1146: Table 'rustdesk_api.tb_x' doesn't exist"
	Fail(c, 500, detail)

	assertGeneric500Body(t, w, logBuf, detail)
}

// TestFail_ServerError_500_LogsOriginalMessageServerSide
// 任务项 4（服务端日志留痕）：>=500 时原始 message 应写入 global.Logger.Error。
func TestFail_ServerError_500_LogsOriginalMessageServerSide(t *testing.T) {
	c, _, logBuf := newTestContext()

	detail := "读取配置文件失败：open /etc/secret: permission denied"
	Fail(c, 500, detail)

	if !strings.Contains(logBuf.String(), detail) {
		t.Errorf(">=500 的原始 message 未记录到服务端日志: %s", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "server error response suppressed for client") {
		t.Errorf("日志应标明该 message 已被客户端侧抑制: %s", logBuf.String())
	}
}

// TestFail_ClientError_400_PassesThroughMessage
// 任务项 3（负向用例）：code<500 时 message 必须原样透传，校验提示不能丢失。
func TestFail_ClientError_400_PassesThroughMessage(t *testing.T) {
	c, w, _ := newTestContext()

	msg := "参数错误：name 不能为空"
	Fail(c, 400, msg)

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v (body=%s)", err, w.Body.String())
	}
	if body["code"] != float64(400) {
		t.Errorf("期望 code=400, 实际 %v", body["code"])
	}
	if body["message"] != msg {
		t.Errorf("期望 message 原样透传 %q, 实际 %v", msg, body["message"])
	}
}

// TestFail_ServerError_500_EmptyMessage_ReturnsGeneric
// 任务项 5（边界）：调用方传入空 message 的 500，仍应返回通用文案，不得 panic / 不得空 body。
func TestFail_ServerError_500_EmptyMessage_ReturnsGeneric(t *testing.T) {
	c, w, _ := newTestContext()

	Fail(c, 500, "")

	if w.Code == 0 {
		t.Fatalf("未产生任何响应（可能 panic）")
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v (body=%s)", err, w.Body.String())
	}
	if body["message"] != "服务器内部错误" {
		t.Errorf("空 message 的 500 也应返回通用文案, 实际 %v (body=%s)", body["message"], w.Body.String())
	}
	if body["message"] == "" {
		t.Errorf("500 响应 message 不应为空")
	}
}

// TestFail_Boundary_499PassesThrough_500Suppressed
// 边界一致性：恰好 499（<500）透传；恰好 500 抑制。明确 code>=500 的判定边界。
func TestFail_Boundary_499PassesThrough_500Suppressed(t *testing.T) {
	// 499 透传
	c499, w499, _ := newTestContext()
	Fail(c499, 499, "自定义业务失败")
	var b499 map[string]interface{}
	_ = json.Unmarshal(w499.Body.Bytes(), &b499)
	if b499["message"] != "自定义业务失败" {
		t.Errorf("499 应透传 message, 实际 %v", b499["message"])
	}

	// 500 抑制
	c500, w500, _ := newTestContext()
	Fail(c500, 500, "内部错误细节")
	var b500 map[string]interface{}
	_ = json.Unmarshal(w500.Body.Bytes(), &b500)
	if b500["message"] != "服务器内部错误" {
		t.Errorf("500 应返回通用文案, 实际 %v", b500["message"])
	}
}
