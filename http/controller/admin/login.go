package admin

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/controller/api"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	apiReq "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	adResp "github.com/lejianwen/rustdesk-api/v2/http/response/admin"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Login struct {
}

// Login 登录
// @Tags 登录
// @Summary 登录
// @Description 登录
// @Accept  json
// @Produce  json
// @Param body body admin.Login true "登录信息"
// @Success 200 {object} response.Response{data=admin.LoginPayload}
// @Failure 500 {object} response.Response
// @Router /admin/login [post]
// @Security token
func (ct *Login) Login(c *gin.Context) {
	if global.Config.App.DisablePwdLogin {
		response.Fail(c, 101, response.TranslateMsg(c, "PwdLoginDisabled"))
		return
	}

	// 检查登录限制
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()
	_, needCaptcha := loginLimiter.CheckSecurityStatus(clientIp)

	f := &admin.Login{}
	err := c.ShouldBindJSON(f)
	if err != nil {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "ParamsError", c.RemoteIP(), clientIp))
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "ParamsError", c.RemoteIP(), clientIp))
		response.Fail(c, 101, errList[0])
		return
	}

	// 检查是否需要验证码
	if needCaptcha {
		if f.CaptchaId == "" || f.Captcha == "" || !loginLimiter.VerifyCaptcha(f.CaptchaId, f.Captcha) {
			response.Fail(c, 101, response.TranslateMsg(c, "CaptchaError"))
			return
		}
	}

	u := service.AllService.UserService.InfoByUsernamePassword(f.Username, f.Password)

	if u.Id == 0 {
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "UsernameOrPasswordError", c.RemoteIP(), clientIp))
		loginLimiter.RecordFailedAttempt(clientIp)
		if _, needCaptcha = loginLimiter.CheckSecurityStatus(clientIp); needCaptcha {
			response.Fail(c, 110, response.TranslateMsg(c, "UsernameOrPasswordError"))
		} else {
			response.Fail(c, 101, response.TranslateMsg(c, "UsernameOrPasswordError"))
		}
		return
	}

	if !service.AllService.UserService.CheckUserEnable(u) {
		if needCaptcha {
			response.Fail(c, 110, response.TranslateMsg(c, "UserDisabled"))
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "UserDisabled"))
		return
	}

	// MFA 二次验证：已启用则下发临时令牌，前端进入动态码输入步骤
	if u.MfaEnabled {
		mfaToken := global.Jwt.GenerateMfaToken(u.Id)
		global.Logger.Infof("[MFA] Login() uid=%d mfa_token_len=%d", u.Id, len(mfaToken))
		response.SendResponse(c, 113, response.TranslateMsg(c, "MfaRequired"), gin.H{"mfa_token": mfaToken})
		return
	}

	ut := service.AllService.UserService.Login(u, &model.LoginLog{
		UserId:   u.Id,
		Client:   model.LoginLogClientWebAdmin,
		Uuid:     "", //must be empty
		Ip:       clientIp,
		Type:     model.LoginLogTypeAccount,
		Platform: f.Platform,
	})

	// 登录成功，清除登录限制
	loginLimiter.RemoveAttempts(clientIp)
	responseLoginSuccess(c, u, ut.Token)
}
// MfaLogin MFA 二次验证后签发正式令牌
// @Tags 登录
// @Summary MFA 二次验证登录
// @Description 使用登录时返回的 mfa_token 与动态码/恢复码换取正式登录令牌
// @Accept  json
// @Produce  json
// @Param body body admin.MfaLogin true "MFA 验证信息"
// @Success 200 {object} response.Response{data=admin.LoginPayload}
// @Failure 500 {object} response.Response
// @Router /admin/login/mfa [post]
func (ct *Login) MfaLogin(c *gin.Context) {
	f := &admin.MfaLogin{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	// DEBUG: 记录原始 body 结构（仅字段名和长度，不记录敏感值），用于排查前端未传 mfa_token 的问题
	var rawBody map[string]interface{}
	_ = c.ShouldBindJSON(&rawBody)
	fieldLog := make([]string, 0, 4)
	for k, v := range rawBody {
		switch val := v.(type) {
		case string:
			fieldLog = append(fieldLog, fmt.Sprintf("%s(len=%d)", k, len(val)))
		default:
			fieldLog = append(fieldLog, fmt.Sprintf("%s(%T)", k, val))
		}
	}
	global.Logger.Infof("[MFA] MfaLogin raw_body=[%s] parsed={MfaToken=%q HasCode=%t HasRecoveryCode=%t Platform=%q}",
		strings.Join(fieldLog, ", "), f.MfaToken, f.Code != "", f.RecoveryCode != "", f.Platform)

	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		// mfa_token 彻底缺失：返回专用错误码 114 引导前端重新走登录流程
		if f.MfaToken == "" {
			headerToken := c.GetHeader("X-Mfa-Token")
			if headerToken != "" {
				global.Logger.Infof("[MFA] MfaLogin: body mfa_token empty, fallback to X-Mfa-Token header")
				f.MfaToken = headerToken
				goto verify
			}
			global.Logger.Warnf("[MFA] MfaLogin: mfa_token missing from both body and header, raw fields=%v", fieldLog)
			response.Fail(c, 114, response.TranslateMsg(c, "MfaTokenMissing"))
			return
		}
		global.Logger.Warnf("[MFA] MfaLogin: validation failed: %v", errList)
		response.Fail(c, 101, errList[0])
		return
	}
verify:
	uid, err := global.Jwt.ParseMfaToken(f.MfaToken)
	if err != nil {
		// SECURITY: MFA 临时令牌短时效(5分钟)且敏感，日志中只记录长度，避免泄露可被重放的令牌。
		global.Logger.Warnf("[MFA] MfaLogin: ParseMfaToken failed, mfa_token_len=%d err=%v", len(f.MfaToken), err)
		response.Fail(c, 101, response.TranslateMsg(c, "MfaTokenInvalid"))
		return
	}
	u := service.AllService.UserService.InfoById(uid)
	if u.Id == 0 || !u.MfaEnabled {
		response.Fail(c, 101, response.TranslateMsg(c, "MfaTokenInvalid"))
		return
	}
	// 校验动态码或恢复码
	ok := false
	if f.RecoveryCode != "" {
		ok = service.AllService.UserService.VerifyMfaRecovery(u, f.RecoveryCode)
	} else {
		ok = service.AllService.UserService.VerifyMfaCode(u, f.Code)
	}
	if !ok {
		response.Fail(c, 101, response.TranslateMsg(c, "MfaCodeError"))
		return
	}
	ut := service.AllService.UserService.Login(u, &model.LoginLog{
		UserId:   u.Id,
		Client:   model.LoginLogClientWebAdmin,
		Uuid:     "", //must be empty
		Ip:       c.ClientIP(),
		Type:     model.LoginLogTypeAccount,
		Platform: f.Platform,
	})
	responseLoginSuccess(c, u, ut.Token)
}

func (ct *Login) Captcha(c *gin.Context) {
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()
	banned, needCaptcha := loginLimiter.CheckSecurityStatus(clientIp)
	if banned {
		response.Fail(c, 101, response.TranslateMsg(c, "LoginBanned"))
		return
	}
	if !needCaptcha {
		response.Fail(c, 101, response.TranslateMsg(c, "NoCaptchaRequired"))
		return
	}
	err, captcha := loginLimiter.RequireCaptcha()
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "CaptchaError")+err.Error())
		return
	}
	err, b64 := loginLimiter.DrawCaptcha(captcha.Content)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "CaptchaError")+err.Error())
		return
	}
	response.Success(c, gin.H{
		"captcha": gin.H{
			"id":  captcha.Id,
			"b64": b64,
		},
	})
}

// Logout 登出
// @Tags 登录
// @Summary 登出
// @Description 登出
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/logout [post]
func (ct *Login) Logout(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	token, ok := c.Get("token")
	if ok {
		service.AllService.UserService.Logout(u, token.(string))
	}
	response.Success(c, nil)
}

// LoginOptions
// @Tags 登录
// @Summary 登录选项
// @Description 登录选项
// @Accept  json
// @Produce  json
// @Success 200 {object} []string
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/login-options [post]
func (ct *Login) LoginOptions(c *gin.Context) {
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()
	banned, needCaptcha := loginLimiter.CheckSecurityStatus(clientIp)
	if banned {
		response.Fail(c, 101, response.TranslateMsg(c, "LoginBanned"))
		return
	}
	ops := service.AllService.OauthService.GetOauthProviders()
	response.Success(c, gin.H{
		"ops":          ops,
		"register":     global.Config.App.Register,
		"need_captcha": needCaptcha,
		"disable_pwd":  global.Config.App.DisablePwdLogin,
		"auto_oidc":    global.Config.App.DisablePwdLogin && len(ops) == 1,
	})
}

// OidcAuth
// @Tags Oauth
// @Summary OidcAuth
// @Description OidcAuth
// @Accept  json
// @Produce  json
// @Router /admin/oidc/auth [post]
func (ct *Login) OidcAuth(c *gin.Context) {
	// o := &api.Oauth{}
	// o.OidcAuth(c)
	f := &apiReq.OidcAuthRequest{}
	err := c.ShouldBindJSON(f)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	err, state, verifier, nonce, url := service.AllService.OauthService.BeginAuth(f.Op)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, err.Error()))
		return
	}

	service.AllService.OauthService.SetOauthCache(state, &service.OauthCacheItem{
		Action:     service.OauthActionTypeLogin,
		Op:         f.Op,
		Id:         f.Id,
		DeviceType: "webadmin",
		// DeviceOs: ct.Platform(c),
		DeviceOs: f.DeviceInfo.Os,
		Uuid:     f.Uuid,
		Verifier: verifier,
		Nonce:    nonce,
	}, 5*60)

	response.Success(c, gin.H{
		"code": state,
		"url":  url,
	})
}

// OidcAuthQuery
// @Tags Oauth
// @Summary OidcAuthQuery
// @Description OidcAuthQuery
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response{data=admin.LoginPayload}
// @Failure 500 {object} response.Response
// @Router /admin/oidc/auth-query [get]
func (ct *Login) OidcAuthQuery(c *gin.Context) {
	o := &api.Oauth{}
	u, ut := o.OidcAuthQueryPre(c)
	if ut == nil {
		return
	}
	responseLoginSuccess(c, u, ut.Token)
}

func responseLoginSuccess(c *gin.Context, u *model.User, token string) {
	lp := &adResp.LoginPayload{}
	lp.FromUser(u)
	lp.Token = token
	lp.RouteNames = service.AllService.UserService.RouteNames(u)
	response.Success(c, lp)
}
