package response

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"net/http"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}
type PageData struct {
	Page  int         `json:"page"`
	Total int         `json:"total"`
	List  interface{} `json:"list"`
}

type DataResponse struct {
	Total uint        `json:"total"`
	Data  interface{} `json:"data"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func SendResponse(c *gin.Context, code int, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		code, message, data,
	})
}

func Success(c *gin.Context, data interface{}) {
	SendResponse(c, 0, "success", data)
}

// Fail 返回失败响应（HTTP 200 + JSON 体中携带业务 code/message/data，与全站约定一致）。
//
// 安全收口（统一 500 错误响应）：当 code >= 500（服务端错误）时，无论调用方传入何种
// message，一律不向客户端回显内部错误细节（可能包含 SQL、表名、文件路径、堆栈等敏感信息），
// 仅返回统一通用文案 "服务器内部错误"，并把调用方传入的原始 message 仅记录到服务端日志，
// 以便排障。4xx 等客户端错误仍按原样透传 message（如参数校验提示）。
func Fail(c *gin.Context, code int, message string) {
	if code >= 500 {
		if message != "" {
			// 仅记录到服务端日志，绝不给客户端回显。
			global.Logger.Error("server error response suppressed for client: " + message)
		}
		message = "服务器内部错误"
	}
	SendResponse(c, code, message, nil)
}

func Error(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Error: message,
	})
}

// ServerError 以统一通用 5xx 包络返回（HTTP 200 + 业务 code 500 + message），
// 避免向客户端泄露内部错误细节（如 SQL、堆栈信息）。
// 供控制器在捕获到非预期的服务端错误时调用，替代直接把 err.Error() 回显给客户端。
func ServerError(c *gin.Context) {
	Fail(c, 500, "")
}

type ServerConfigResponse struct {
	IdServer    string `json:"id_server"`
	Key         string `json:"key"`
	RelayServer string `json:"relay_server"`
	ApiServer   string `json:"api_server"`
}

func TranslateMsg(c *gin.Context, messageId string) string {
	localizer := global.Localizer(c.GetHeader("Accept-Language"))
	errMsg, err := localizer.LocalizeMessage(&i18n.Message{
		ID: messageId,
	})
	if err != nil {
		global.Logger.Warn("LocalizeMessage Error: " + err.Error())
		errMsg = messageId
	}
	return errMsg
}
func TranslateTempMsg(c *gin.Context, messageId string, templateData map[string]interface{}) string {
	localizer := global.Localizer(c.GetHeader("Accept-Language"))
	errMsg, err := localizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID: messageId,
		},
		TemplateData: templateData,
	})
	if err != nil {
		global.Logger.Warn("LocalizeMessage Error: " + err.Error())
		errMsg = messageId
	}
	return errMsg
}
func TranslateParamMsg(c *gin.Context, messageId string, params ...string) string {
	localizer := global.Localizer(c.GetHeader("Accept-Language"))
	templateData := make(map[string]interface{})
	for i, v := range params {
		k := fmt.Sprintf("P%d", i)
		templateData[k] = v
	}
	errMsg, err := localizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID: messageId,
		},
		TemplateData: templateData,
	})
	if err != nil {
		global.Logger.Warn("LocalizeMessage Error: " + err.Error())
		errMsg = messageId
	}
	return errMsg
}
