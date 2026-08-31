package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
}

// ServerConfig RUSTDESK服务配置
// @Tags ADMIN
// @Summary RUSTDESK服务配置
// @Description 服务配置,给webclient提供api-server
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/server [get]
// @Security token
func (co *Config) ServerConfig(c *gin.Context) {
	cf := &response.ServerConfigResponse{
		IdServer:    global.Config.Rustdesk.IdServer,
		Key:         global.Config.Rustdesk.Key,
		RelayServer: global.Config.Rustdesk.RelayServer,
		ApiServer:   global.Config.Rustdesk.ApiServer,
	}
	response.Success(c, cf)
}

// AppConfig APP服务配置
// @Tags ADMIN
// @Summary APP服务配置
// @Description APP服务配置
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/app [get]
// @Security token
func (co *Config) AppConfig(c *gin.Context) {
	response.Success(c, &gin.H{
		"web_client": global.Config.App.WebClient,
	})
}

// AdminConfig ADMIN服务配置
// @Tags ADMIN
// @Summary ADMIN服务配置
// @Description ADMIN服务配置
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/admin [get]
// @Security token
func (co *Config) AdminConfig(c *gin.Context) {

	u := &model.User{}
	token, _ := c.Cookie("access_token")
	if token != "" {
		u, _ = service.AllService.UserService.InfoByAccessToken(token, "")
		if !service.AllService.UserService.CheckUserEnable(u) {
			u.Id = 0
		}
	}

	if u.Id == 0 {
		response.Success(c, &gin.H{
			"title": global.Config.Admin.Title,
		})
		return
	}

	hello := global.Config.Admin.Hello
	if hello == "" {
		helloFile := global.Config.Admin.HelloFile
		if helloFile != "" {
			b, err := os.ReadFile(helloFile)
			if err == nil && len(b) > 0 {
				hello = string(b)
			}
		}
	}

	//replace {{username}} to username
	hello = strings.Replace(hello, "{{username}}", u.Username, -1)
	response.Success(c, &gin.H{
		"title": global.Config.Admin.Title,
		"hello": hello,
	})
}

// ConfigFileGet 读取后端配置文件（config.yaml）原始内容，仅管理员可用
// @Tags ADMIN
// @Summary 读取后端配置文件
// @Description 读取配置文件原始内容，供前端编辑
// @Produce json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/file/get [get]
// @Security token
func (co *Config) ConfigFileGet(c *gin.Context) {
	path := global.ConfigPath
	if path == "" {
		response.Fail(c, 500, "配置文件路径未知")
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		response.Fail(c, 500, "读取配置文件失败："+err.Error())
		return
	}
	response.Success(c, gin.H{
		"path":    path,
		"content": string(b),
	})
}

// ConfigFileUpdate 保存后端配置文件（config.yaml）原始内容，仅管理员可用
// @Tags ADMIN
// @Summary 保存后端配置文件
// @Description 校验 YAML 后写回配置文件，修改需重启服务生效
// @Accept json
// @Produce json
// @Param body body object{content=string} true "配置文件内容"
// @Success 200 {object} response.Response
// @Failure 101 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/file/update [post]
// @Security token
func (co *Config) ConfigFileUpdate(c *gin.Context) {
	type Req struct {
		Content        string `json:"content" binding:"required"`
		VerifyUsername string `json:"verify_username"` // 超级管理员用户名（操作者自行填写，接口不返回）
		VerifyPassword string `json:"verify_password"` // 超级管理员登录密码
		MfaCode        string `json:"mfa_code"`        // 超级管理员 MFA 动态码（其开启 MFA 时必填）
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		global.Logger.Error("config update bind/parse error: " + err.Error())
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+" 配置格式有误，请检查输入")
		return
	}
	path := global.ConfigPath
	if path == "" {
		response.Fail(c, 500, "配置文件路径未知")
		return
	}
	// 修改配置文件属高危操作：需超级管理员（最早创建的管理员，id=1）账号授权。
	// 与新建/提升管理员的二次确认共用同一逻辑：用户名 + 密码 + MFA，失败统一返回 AdminAuthFailed。
	if ok, errMsg := verifyAdminReconfirm(req.VerifyUsername, req.VerifyPassword, req.MfaCode); !ok {
		response.Fail(c, 101, response.TranslateMsg(c, errMsg))
		return
	}
	// 校验 YAML 合法性（不真正加载到运行配置，仅解析）
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(req.Content)); err != nil {
		global.Logger.Error("config file YAML parse error: " + err.Error())
		response.Fail(c, 101, "配置格式错误(YAML 解析失败)，请检查 YAML 语法")
		return
	}
	if err := os.WriteFile(path, []byte(req.Content), 0644); err != nil {
		response.Fail(c, 500, "写入配置文件失败："+err.Error())
		return
	}
	response.Success(c, nil)
}
