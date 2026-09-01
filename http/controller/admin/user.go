package admin

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	adResp "github.com/lejianwen/rustdesk-api/v2/http/response/admin"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"
	"strconv"
)

type User struct {
}

// verifyAdminReconfirm 创建/提升管理员时的二次确认：
// 操作者必须自行填写超级管理员账户（最早创建的管理员，Role=admin 且 id 最小）的用户名、
// 登录密码；若该账户开启了 MFA 还需校验其动态码。
// 出于防暴力破解考虑，接口不返回超级管理员的用户名，任何校验失败统一返回
// "AdminAuthFailed"（不区分用户名/密码/MFA 哪个出错），避免攻击者枚举有效账号。
func verifyAdminReconfirm(verifyUsername, verifyPassword, mfaCode string) (bool, string) {
	root := service.AllService.UserService.RootAdmin()
	if root == nil {
		return false, "AdminAuthFailed"
	}
	// 用户名必须与超级管理员一致（大小写不敏感）
	if !strings.EqualFold(strings.TrimSpace(verifyUsername), root.Username) {
		return false, "AdminAuthFailed"
	}
	// 校验超级管理员密码
	ok, _, err := utils.VerifyPassword(root.Password, verifyPassword)
	if err != nil || !ok {
		return false, "AdminAuthFailed"
	}
	// 若超级管理员开启了 MFA，必须校验其动态码
	if root.MfaEnabled {
		if !service.AllService.UserService.VerifyMfaCode(root, mfaCode) {
			return false, "AdminAuthFailed"
		}
	}
	return true, ""
}

// Detail 管理员
// @Tags 用户
// @Summary 管理员详情
// @Description 管理员详情
// @Accept  json
// @Produce  json
// @Param id path int true "ID"
// @Success 200 {object} response.Response{data=model.User}
// @Failure 500 {object} response.Response
// @Router /admin/user/detail/{id} [get]
// @Security token
func (ct *User) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	u := service.AllService.UserService.InfoById(uint(iid))
	if u.Id > 0 {
		response.Success(c, u)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// Create 管理员
// @Tags 用户
// @Summary 创建管理员
// @Description 创建管理员
// @Accept  json
// @Produce  json
// @Param body body admin.UserForm true "管理员信息"
// @Success 200 {object} response.Response{data=model.User}
// @Failure 500 {object} response.Response
// @Router /admin/user/create [post]
// @Security token
func (ct *User) Create(c *gin.Context) {
	f := &admin.UserForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	// 手动创建用户必须设置初始密码，否则用户无法登录
	if strings.TrimSpace(f.Password) == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "PasswordRequired"))
		return
	}
	u := f.ToUser()
	// 创建时设置初始密码
	u.Password = f.Password
	// 兼容旧字段：设置了 is_admin=true 但未传 role 时同步
	if u.Role == "" && u.IsAdmin != nil && *u.IsAdmin {
		u.Role = "admin"
	} else if u.Role == "" {
		u.Role = "user"
	}
	// 新建管理员：必须超级管理员账户二次确认（其用户名 + 密码 + MFA）
	if u.Role == "admin" {
		if ok, errMsg := verifyAdminReconfirm(f.VerifyUsername, f.VerifyPassword, f.MfaCode); !ok {
			response.Fail(c, 101, response.TranslateMsg(c, errMsg))
			return
		}
	}
	err := service.AllService.UserService.Create(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	// 记录管理员新建账户审计
	cur := service.AllService.UserService.CurUser(c)
	opId := uint(0)
	opName := ""
	if cur != nil {
		opId = cur.Id
		opName = cur.Username
	}
	_ = service.AllService.AccountOpLogService.Log(opId, opName, u.Id, u.Username, "create", "管理员创建账户")
	response.Success(c, nil)
}

// List 列表
// @Tags 用户
// @Summary 管理员列表
// @Description 管理员列表
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Param username query int false "账户"
// @Success 200 {object} response.Response{data=model.UserList}
// @Failure 500 {object} response.Response
// @Router /admin/user/list [get]
// @Security token
func (ct *User) List(c *gin.Context) {
	query := &admin.UserQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.UserService.List(query.Page, query.PageSize, func(tx *gorm.DB) {
		if query.Username != "" {
			tx.Where("username like ?", "%"+query.Username+"%")
		}
		// 按部门筛选时，包含其所有子部门下的用户
		if query.GroupId > 0 {
			ids := service.AllService.GroupService.DescendantIds(query.GroupId)
			ids = append(ids, query.GroupId)
			tx.Where("group_id in ?", ids)
		}
	})
	response.Success(c, res)
}

// Update 编辑
// @Tags 用户
// @Summary 管理员编辑
// @Description 管理员编辑
// @Accept  json
// @Produce  json
// @Param body body admin.UserForm true "用户信息"
// @Success 200 {object} response.Response{data=model.User}
// @Failure 500 {object} response.Response
// @Router /admin/user/update [post]
// @Security token
func (ct *User) Update(c *gin.Context) {
	f := &admin.UserForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := f.ToUser()
	// 兼容旧字段：设置了 is_admin=true 但未传 role 时同步
	if u.Role == "" && u.IsAdmin != nil && *u.IsAdmin {
		u.Role = "admin"
	} else if u.Role == "" {
		u.Role = "user"
	}
	// 将普通用户提升为管理员：必须超级管理员账户二次确认（其用户名 + 密码 + MFA）
	oldUser := service.AllService.UserService.InfoById(f.Id)
	if u.Role == "admin" && (oldUser.Id == 0 || oldUser.Role != "admin") {
		if ok, errMsg := verifyAdminReconfirm(f.VerifyUsername, f.VerifyPassword, f.MfaCode); !ok {
			response.Fail(c, 101, response.TranslateMsg(c, errMsg))
			return
		}
	}
	err := service.AllService.UserService.Update(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	recordOpLog(c, u, "update", "管理员编辑账户")
	response.Success(c, nil)
}

// Delete 删除
// @Tags 用户
// @Summary 管理员删除
// @Description 管理员编删除
// @Accept  json
// @Produce  json
// @Param body body admin.UserForm true "用户信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/user/delete [post]
// @Security token
func (ct *User) Delete(c *gin.Context) {
	f := &admin.UserForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id := f.Id
	errList := global.Validator.ValidVar(c, id, "required,gt=0")
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.InfoById(f.Id)
	if u.Id > 0 {
		err := service.AllService.UserService.Delete(u)
		if err == nil {
			recordOpLog(c, u, "delete", "管理员删除账户")
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, err.Error())
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// UpdatePassword 修改密码
// @Tags 用户
// @Summary 修改密码
// @Description 修改密码
// @Accept  json
// @Produce  json
// @Param body body admin.UserPasswordForm true "用户信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/user/updatePassword [post]
// @Security token
func (ct *User) UpdatePassword(c *gin.Context) {
	f := &admin.UserPasswordForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.InfoById(f.Id)
	if u.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.UserService.UpdatePassword(u, f.Password)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	// 改密码后清除该用户所有会话令牌，强制重新登录，防止旧 token 被冒用
	_ = service.AllService.UserService.FlushToken(u)
	recordOpLog(c, u, "update", "管理员重置账户密码")
	response.Success(c, nil)
}

// Current 当前用户
// @Tags 用户
// @Summary 当前用户
// @Description 当前用户
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response{data=admin.LoginPayload}
// @Failure 500 {object} response.Response
// @Router /admin/user/current [get]
// @Security token
func (ct *User) Current(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	lp := &adResp.LoginPayload{}
	lp.FromUser(u)
	lp.Token = "" // 不向前端暴露 token（已通过 HttpOnly Cookie 下发）
	lp.RouteNames = service.AllService.UserService.RouteNames(u)
	response.Success(c, lp)
}

// ChangeCurPwd 修改当前用户密码
// @Tags 用户
// @Summary 修改当前用户密码
// @Description 修改当前用户密码
// @Accept  json
// @Produce  json
// @Param body body admin.ChangeCurPasswordForm true "用户信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/user/changeCurPwd [post]
// @Security token
func (ct *User) ChangeCurPwd(c *gin.Context) {
	f := &admin.ChangeCurPasswordForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.CurUser(c)
	// Verify the old password only when the account already has one set
	if !service.AllService.UserService.IsPasswordEmptyByUser(u) {
		ok, _, err := utils.VerifyPassword(u.Password, f.OldPassword)
		if err != nil || !ok {
			response.Fail(c, 101, response.TranslateMsg(c, "OldPasswordError"))
			return
		}
	}
	err := service.AllService.UserService.UpdatePassword(u, f.NewPassword)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	// 改密码后清除当前用户所有会话令牌，强制重新登录，防止旧 token 被冒用
	_ = service.AllService.UserService.FlushToken(u)
	recordOpLog(c, u, "update", "用户修改自己密码")
	response.Success(c, nil)
}

// MyOauth
// @Tags 用户
// @Summary 我的授权
// @Description 我的授权
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response{data=[]admin.UserOauthItem}
// @Failure 500 {object} response.Response
// @Router /admin/user/myOauth [get]
// @Security token
func (ct *User) MyOauth(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	oal := service.AllService.OauthService.List(1, 100, nil)
	ops := make([]string, 0)
	for _, oa := range oal.Oauths {
		ops = append(ops, oa.Op)
	}
	uts := service.AllService.UserService.UserThirdsByUserId(u.Id)
	var res []*adResp.UserOauthItem
	for _, oa := range oal.Oauths {
		item := &adResp.UserOauthItem{
			Op: oa.Op,
		}
		for _, ut := range uts {
			if ut.Op == oa.Op {
				item.Status = 1
				break
			}
		}
		res = append(res, item)
	}
	response.Success(c, res)
}

// ===================== MFA (TOTP) =====================

// MfaSetup 生成 TOTP 密钥与二维码，暂存密钥（尚未启用）
// @Tags 用户
// @Summary MFA 初始化
// @Router /admin/user/mfa/setup [post]
// @Security token
func (ct *User) MfaSetup(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "RustDesk",
		AccountName: u.Username,
	})
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	if err := service.AllService.UserService.SetMfaSecret(u, key.Secret()); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	qrPng, qerr := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	qr := ""
	if qerr == nil {
		qr = "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPng)
	}
	response.Success(c, gin.H{
		"secret":      key.Secret(),
		"otpauth_url": key.URL(),
		"qr":          qr,
	})
}

// MfaEnable 校验动态码后启用 MFA，返回一次性恢复码
// @Tags 用户
// @Summary MFA 启用
// @Router /admin/user/mfa/enable [post]
// @Security token
func (ct *User) MfaEnable(c *gin.Context) {
	f := &admin.MfaEnableForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.CurUser(c)
	if u.MfaSecret == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "MfaSecretEmpty"))
		return
	}
	if !service.AllService.UserService.VerifyMfaCode(u, f.Code) {
		response.Fail(c, 101, response.TranslateMsg(c, "MfaCodeError"))
		return
	}
	codes, err := service.AllService.UserService.EnableMfa(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	recordOpLog(c, u, "update", "用户启用 MFA")
	response.Success(c, gin.H{"recovery_codes": codes})
}

// MfaDisable 校验登录密码后关闭 MFA
// @Tags 用户
// @Summary MFA 关闭
// @Router /admin/user/mfa/disable [post]
// @Security token
func (ct *User) MfaDisable(c *gin.Context) {
	f := &admin.MfaDisableForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.CurUser(c)
	checked := service.AllService.UserService.InfoByUsernamePassword(u.Username, f.Password)
	if checked.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "PasswordError"))
		return
	}
	if err := service.AllService.UserService.DisableMfa(u); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	// 关闭 MFA 后清除当前用户所有会话令牌，强制重新登录，防止旧 token 被冒用
	_ = service.AllService.UserService.FlushToken(u)
	recordOpLog(c, u, "update", "用户关闭 MFA")
	response.Success(c, nil)
}

// MfaStatus 返回当前用户 MFA 启用状态
// @Tags 用户
// @Summary MFA 状态
// @Router /admin/user/mfa/status [get]
// @Security token
func (ct *User) MfaStatus(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	response.Success(c, gin.H{"mfa_enabled": u.MfaEnabled})
}

// MfaReset 管理员强制关闭指定用户的 MFA（用户丢失验证器/恢复码时的救援手段）
// @Tags 用户
// @Summary 管理员重置用户 MFA
// @Router /admin/user/mfa/reset [post]
// @Security token
func (ct *User) MfaReset(c *gin.Context) {
	f := &admin.MfaResetForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.InfoById(f.UserId)
	if u.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if err := service.AllService.UserService.DisableMfa(u); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	// 重置 MFA 后清除该用户所有会话令牌，强制重新登录，防止旧 token 被冒用
	_ = service.AllService.UserService.FlushToken(u)
	recordOpLog(c, u, "update", "管理员重置用户 MFA")
	response.Success(c, nil)
}

// groupUsers
func (ct *User) GroupUsers(c *gin.Context) {
	aG := service.AllService.GroupService.List(1, 999, nil)
	aU := service.AllService.UserService.List(1, 9999, nil)
	response.Success(c, gin.H{
		"groups": aG.Groups,
		"users":  aU.Users,
	})
}

// Register
func (ct *User) Register(c *gin.Context) {
	if !global.Config.App.Register {
		response.Fail(c, 101, response.TranslateMsg(c, "RegisterClosed"))
		return
	}
	f := &admin.RegisterForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}

	// 授权码验证：当开启了邀请模式时，必须提供有效授权码
	// 授权码同时控制注册资格 + 订阅激活
	var userExpiredAt int64
	if global.Config.App.InviteOnly {
		if f.InviteCode == "" {
			response.Fail(c, 101, response.TranslateMsg(c, "InviteCodeRequired"))
			return
		}
		// 用 InviteCode（授权码）替代旧的 Invitation
		ics := &service.InviteCodeService{}
		ic := ics.InfoByCode(f.InviteCode)
		if ic == nil || ic.Id == 0 {
			response.Fail(c, 101, response.TranslateMsg(c, "InviteCodeInvalid"))
			return
		}
		if ic.Status != "unused" {
			response.Fail(c, 101, response.TranslateMsg(c, "InviteCodeInvalid"))
			return
		}
		if ic.ExpireAt.Before(time.Now()) {
			response.Fail(c, 101, response.TranslateMsg(c, "InviteCodeInvalid"))
			return
		}
	}

	regStatus := model.StatusCode(global.Config.App.RegisterStatus)
	// 注册状态可能未配置，默认启用
	if regStatus != model.COMMON_STATUS_DISABLED && regStatus != model.COMMON_STATUS_ENABLE {
		regStatus = model.COMMON_STATUS_ENABLE
	}
	// 通过授权码注册的用户：直接启用，无需管理员手动启用账户
	if global.Config.App.InviteOnly && f.InviteCode != "" {
		regStatus = model.COMMON_STATUS_ENABLE
	}

	u := service.AllService.UserService.Register(f.Username, f.Email, f.Password, regStatus, userExpiredAt)
	if u == nil || u.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed"))
		return
	}

	// 注册成功后消耗授权码 + 激活订阅（原子更新防止并发重复使用）
	if global.Config.App.InviteOnly && f.InviteCode != "" {
		ics := &service.InviteCodeService{}
		ic := ics.InfoByCode(f.InviteCode)
		if ic != nil && ic.Id > 0 && ic.Status == "unused" {
			now := time.Now()
			// 原子更新：只更新 status="unused" 的记录
			result := global.DB.Model(&model.InviteCode{}).
				Where("id = ? AND status = ?", ic.Id, "unused").
				Updates(map[string]interface{}{
					"status":  "used",
					"used_by": u.Id,
					"used_at": &now,
				})
			if result.Error != nil || result.RowsAffected == 0 {
				// 原子更新失败（已被别人使用），删除已创建的用户
				global.DB.Delete(&model.User{}, u.Id)
				response.Fail(c, 101, response.TranslateMsg(c, "InviteCodeInvalid"))
				return
			}
			// 授权码绑定付费会员时长
			var newExpire time.Time
			if ic.IsForever() {
				// 永久授权码：会员设为 9999 年
				newExpire = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
			} else {
				periodDuration := time.Duration(ic.ExpireDays*24) * time.Hour
				newExpire = now.Add(periodDuration)
			}
			// 只更新订阅时长；expired_at 保持 0（永不过期），
			// 保证订阅到期后用户仍可登录后台续费。
			global.DB.Model(&model.User{}).Where("id = ?", u.Id).
				Updates(map[string]interface{}{
					"subscription_plan":      ic.Plan,
					"subscription_expire_at": &newExpire,
				})
		}
	}

	// 登录后台与远程连接是两种独立情况：
	// 1. 注册用户账户 expired_at=0（永不过期），可正常登录后台；
	// 2. 远程连接必须付费会员且在有效期内（subscription_expire_at 有效），
	//    未付费用户 subscription_expire_at 为 nil（status=none），客户端会拒绝连接。
	// 因此注册时不再默认设置永久订阅，仅当使用授权码激活时才绑定订阅时长。

	// 记录账户新建操作审计
	_ = service.AllService.AccountOpLogService.Log(u.Id, u.Username, u.Id, u.Username, "create", "用户注册新建账户")

	if regStatus == model.COMMON_STATUS_DISABLED {
		// 需要管理员审核
		response.Fail(c, 101, response.TranslateMsg(c, "RegisterSuccessWaitAdminConfirm"))
		return
	}
	// 注册成功后自动登录
	ut := service.AllService.UserService.Login(u, &model.LoginLog{
		UserId: u.Id,
		Client: model.LoginLogClientWebAdmin,
		Uuid:   "",
		Ip:     c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Type:   model.LoginLogTypeAccount,
	})
	responseLoginSuccess(c, u, ut)
}
