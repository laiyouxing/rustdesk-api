package service

import (
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
)

type UserService struct {
}

// InfoById 根据用户id取用户信息
func (us *UserService) InfoById(id uint) *model.User {
	u := &model.User{}
	DB.Where("id = ?", id).First(u)
	return u
}

// InfoByUsername 根据用户名取用户信息
func (us *UserService) InfoByUsername(un string) *model.User {
	u := &model.User{}
	DB.Where("username = ?", un).First(u)
	return u
}

// InfoByEmail 根据邮箱取用户信息
func (us *UserService) InfoByEmail(email string) *model.User {
	u := &model.User{}
	DB.Where("email = ?", email).First(u)
	return u
}

// InfoByOpenid 根据openid取用户信息
func (us *UserService) InfoByOpenid(openid string) *model.User {
	u := &model.User{}
	DB.Where("openid = ?", openid).First(u)
	return u
}

// InfoByUsernamePassword 根据用户名密码取用户信息
func (us *UserService) InfoByUsernamePassword(username, password string) *model.User {
	if Config.Ldap.Enable {
		u, err := AllService.LdapService.Authenticate(username, password)
		if err == nil {
			return u
		}
		Logger.Errorf("LDAP authentication failed, %v", err)
		Logger.Warn("Fallback to local database")
	}
	u := &model.User{}
	DB.Where("username = ?", username).First(u)
	if u.Id == 0 {
		return u
	}
	ok, newHash, err := utils.VerifyPassword(u.Password, password)
	if err != nil || !ok {
		return &model.User{}
	}
	if newHash != "" {
		DB.Model(u).Update("password", newHash)
		u.Password = newHash
	}
	return u
}

// InfoByAccesstoken 根据accesstoken取用户信息
// fingerprint 为调用方计算出的来源指纹（IP+User-Agent 哈希）；传空字符串表示跳过校验
// （用于客户端 rustauth 及公开接口，这些场景不绑定来源）。
func (us *UserService) InfoByAccessToken(token string, fingerprint string) (*model.User, *model.UserToken) {
	u := &model.User{}
	ut := &model.UserToken{}
	DB.Where("token = ?", token).First(ut)
	if ut.Id == 0 {
		return u, ut
	}
	if ut.ExpiredAt < time.Now().Unix() {
		return u, ut
	}
	// 来源指纹校验：当调用方提供了指纹且记录中也存在指纹时，必须一致，
	// 否则视为 token 被异地/异设备盗用，拒绝放行（需重新登录）。
	if fingerprint != "" && ut.Fingerprint != "" && fingerprint != ut.Fingerprint {
		return u, ut
	}
	DB.Where("id = ?", ut.UserId).First(u)
	return u, ut
}

// GenerateToken 生成token
func (us *UserService) GenerateToken(u *model.User) string {
	if len(Jwt.Key) > 0 {
		return Jwt.GenerateToken(u.Id)
	}
	return utils.Md5(u.Username + time.Now().String())
}

// Login 登录
func (us *UserService) Login(u *model.User, llog *model.LoginLog) *model.UserToken {
	token := us.GenerateToken(u)
	// 计算来源指纹（仅 User-Agent），用于后续校验请求来源，防止 token 被盗用。
	// 注意：不使用 IP，因为在 nginx 反代 / IPv4-IPv6 双栈下，同一会话不同请求的
	// c.ClientIP() 会变化，导致指纹误判为异地盗用而被拒（403）。
	fp := utils.Md5(llog.UserAgent)
	ut := &model.UserToken{
		UserId:     u.Id,
		Token:      token,
		DeviceUuid: llog.Uuid,
		DeviceId:   llog.DeviceId,
		ExpiredAt:  us.UserTokenExpireTimestamp(),
		Fingerprint: fp,
	}
	DB.Create(ut)
	llog.UserTokenId = ut.UserId
	DB.Create(llog)
	if llog.Uuid != "" {
		AllService.PeerService.UuidBindUserId(llog.DeviceId, llog.Uuid, u.Id)
	}
	return ut
}

// CurUser 获取当前用户
func (us *UserService) CurUser(c *gin.Context) *model.User {
	user, _ := c.Get("curUser")
	u, ok := user.(*model.User)
	if !ok {
		return nil
	}
	return u
}

func (us *UserService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.UserList) {
	res = &model.UserList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.User{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Users)
	return
}

func (us *UserService) ListByIds(ids []uint) (res []*model.User) {
	DB.Where("id in ?", ids).Find(&res)
	return res
}

// ListByGroupId 根据组id取用户列表
func (us *UserService) ListByGroupId(groupId, page, pageSize uint) (res *model.UserList) {
	res = us.List(page, pageSize, func(tx *gorm.DB) {
		tx.Where("group_id = ?", groupId)
	})
	return
}

// ListIdsByGroupId 根据组id取用户id列表
func (us *UserService) ListIdsByGroupId(groupId uint) (ids []uint) {
	DB.Model(&model.User{}).Where("group_id = ?", groupId).Pluck("id", &ids)
	return ids

}

// ListIdAndNameByGroupId 根据组id取用户id和用户名列表
func (us *UserService) ListIdAndNameByGroupId(groupId uint) (res []*model.User) {
	DB.Model(&model.User{}).Where("group_id = ?", groupId).Select("id, username").Find(&res)
	return res
}

// CheckUserEnable 判断用户是否禁用
func (us *UserService) CheckUserEnable(u *model.User) bool {
	return u.Status == model.COMMON_STATUS_ENABLE
}

// Create 创建
func (us *UserService) Create(u *model.User) error {
	// The initial username should be formatted, and the username should be unique
	if us.IsUsernameExists(u.Username) {
		return errors.New("UsernameExists")
	}
	u.Username = us.formatUsername(u.Username)
	var err error
	u.Password, err = utils.EncryptPassword(u.Password)
	if err != nil {
		return err
	}
	res := DB.Create(u).Error
	return res
}

// GetUuidByToken 根据token和user取uuid
func (us *UserService) GetUuidByToken(u *model.User, token string) string {
	ut := &model.UserToken{}
	err := DB.Where("user_id = ? and token = ?", u.Id, token).First(ut).Error
	if err != nil {
		return ""
	}
	return ut.DeviceUuid
}

// Logout 退出登录 -> 删除token, 解绑uuid
func (us *UserService) Logout(u *model.User, token string) error {
	uuid := us.GetUuidByToken(u, token)
	err := DB.Where("user_id = ? and token = ?", u.Id, token).Delete(&model.UserToken{}).Error
	if err != nil {
		return err
	}
	if uuid != "" {
		AllService.PeerService.UuidUnbindUserId(uuid, u.Id)
	}
	return nil
}

// Delete 删除用户和oauth信息
func (us *UserService) Delete(u *model.User) error {
	userCount := us.getAdminUserCount()
	if userCount <= 1 && us.IsAdmin(u) {
		return errors.New("The last admin user cannot be deleted")
	}
	tx := DB.Begin()
	// 删除用户
	if err := tx.Delete(u).Error; err != nil {
		tx.Rollback()
		return err
	}
	// 删除关联的 OAuth 信息
	if err := tx.Where("user_id = ?", u.Id).Delete(&model.UserThird{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	//  删除关联的ab
	if err := tx.Where("user_id = ?", u.Id).Delete(&model.AddressBook{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	//  删除关联的abc
	if err := tx.Where("user_id = ?", u.Id).Delete(&model.AddressBookCollection{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	//  删除关联的abcr
	if err := tx.Where("user_id = ?", u.Id).Delete(&model.AddressBookCollectionRule{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	// 删除关联的peer
	if err := AllService.PeerService.EraseUserId(u.Id); err != nil {
		Logger.Warn("User deleted successfully, but failed to unlink peer.")
		return nil
	}
	return nil
}

// Update 更新
func (us *UserService) Update(u *model.User) error {
	currentUser := us.InfoById(u.Id)
	// 如果当前用户是管理员并且 IsAdmin 不为空，进行检查
	if us.IsAdmin(currentUser) {
		adminCount := us.getAdminUserCount()
		// 如果这是唯一的管理员，确保不能禁用或取消管理员权限
		if adminCount <= 1 && (!us.IsAdmin(u) || u.Status == model.COMMON_STATUS_DISABLED) {
			return errors.New("The last admin user cannot be disabled or demoted")
		}
	}
	return DB.Model(u).Updates(u).Error
}

// FlushToken 清空token
func (us *UserService) FlushToken(u *model.User) error {
	return DB.Where("user_id = ?", u.Id).Delete(&model.UserToken{}).Error
}

// FlushTokenByUuid 清空token
func (us *UserService) FlushTokenByUuid(uuid string) error {
	return DB.Where("device_uuid = ?", uuid).Delete(&model.UserToken{}).Error
}

// FlushTokenByUuids 清空token
func (us *UserService) FlushTokenByUuids(uuids []string) error {
	return DB.Where("device_uuid in (?)", uuids).Delete(&model.UserToken{}).Error
}

// UpdatePassword 更新密码
func (us *UserService) UpdatePassword(u *model.User, password string) error {
	var err error
	u.Password, err = utils.EncryptPassword(password)
	if err != nil {
		return err
	}
	err = DB.Model(u).Update("password", u.Password).Error
	if err != nil {
		return err
	}
	err = us.FlushToken(u)
	return err
}

// IsAdmin 是否管理员
func (us *UserService) IsAdmin(u *model.User) bool {
	return u != nil && *u.IsAdmin
}

// RouteNames
func (us *UserService) RouteNames(u *model.User) []string {
	if us.IsAdmin(u) {
		return model.AdminRouteNames
	}
	return model.UserRouteNames
}

// InfoByOauthId 根据oauth的name和openId取用户信息
func (us *UserService) InfoByOauthId(op string, openId string) *model.User {
	ut := AllService.OauthService.UserThirdInfo(op, openId)
	if ut.Id == 0 {
		return nil
	}
	u := us.InfoById(ut.UserId)
	if u.Id == 0 {
		return nil
	}
	return u
}

// RegisterByOauth 注册
func (us *UserService) RegisterByOauth(oauthUser *model.OauthUser, op string) (error, *model.User) {
	Lock.Lock("registerByOauth")
	defer Lock.UnLock("registerByOauth")
	ut := AllService.OauthService.UserThirdInfo(op, oauthUser.OpenId)
	if ut.Id != 0 {
		return nil, us.InfoById(ut.UserId)
	}
	err, oauthType := AllService.OauthService.GetTypeByOp(op)
	if err != nil {
		return err, nil
	}
	//check if this email has been registered
	email := oauthUser.Email
	// only email is not empty
	if email != "" {
		email = strings.ToLower(email)
		// update email to oauthUser, in case it contain upper case
		oauthUser.Email = email
		// call this, if find user by email, it will update the email to local database
		user, ldapErr := AllService.LdapService.GetUserInfoByEmailLocal(email)
		// If we enable ldap, and the error is not ErrLdapUserNotFound, return the error because we could not sure if the user is not found in ldap
		if !(errors.Is(ldapErr, ErrLdapNotEnabled) || errors.Is(ldapErr, ErrLdapUserNotFound) || ldapErr == nil) {
			return ldapErr, user
		}
		if user.Id == 0 {
			// this means the user is not found in ldap, maybe ldao is not enabled
			user = us.InfoByEmail(email)
		}
		if user.Id != 0 {
			ut.FromOauthUser(user.Id, oauthUser, oauthType, op)
			DB.Create(ut)
			return nil, user
		}
	}

	tx := DB.Begin()
	ut = &model.UserThird{}
	ut.FromOauthUser(0, oauthUser, oauthType, op)
	// The initial username should be formatted
	username := us.formatUsername(oauthUser.Username)
	usernameUnique := us.GenerateUsernameByOauth(username)
	user := &model.User{
		Username: usernameUnique,
		GroupId:  1,
	}
	oauthUser.ToUser(user, false)
	tx.Create(user)
	if user.Id == 0 {
		tx.Rollback()
		return errors.New("OauthRegisterFailed"), user
	}
	ut.UserId = user.Id
	tx.Create(ut)
	tx.Commit()
	return nil, user
}

// GenerateUsernameByOauth 生成用户名
func (us *UserService) GenerateUsernameByOauth(name string) string {
	for us.IsUsernameExists(name) {
		name += strconv.Itoa(rand.Intn(10)) // Append a random digit (0-9)
	}
	return name
}

// UserThirdsByUserId
func (us *UserService) UserThirdsByUserId(userId uint) (res []*model.UserThird) {
	DB.Where("user_id = ?", userId).Find(&res)
	return res
}

func (us *UserService) UserThirdInfo(userId uint, op string) *model.UserThird {
	ut := &model.UserThird{}
	DB.Where("user_id = ? and op = ?", userId, op).First(ut)
	return ut
}

// FindLatestUserIdFromLoginLogByUuid 根据uuid和设备id查找最后登录的用户id
func (us *UserService) FindLatestUserIdFromLoginLogByUuid(uuid string, deviceId string) uint {
	llog := &model.LoginLog{}
	DB.Where("uuid = ? and device_id = ?", uuid, deviceId).Order("id desc").First(llog)
	return llog.UserId
}

// IsPasswordEmptyById 根据用户id判断密码是否为空，主要用于第三方登录的自动注册
func (us *UserService) IsPasswordEmptyById(id uint) bool {
	u := &model.User{}
	if DB.Where("id = ?", id).First(u).Error != nil {
		return false
	}
	return u.Password == ""
}

// IsPasswordEmptyByUsername 根据用户id判断密码是否为空，主要用于第三方登录的自动注册
func (us *UserService) IsPasswordEmptyByUsername(username string) bool {
	u := &model.User{}
	if DB.Where("username = ?", username).First(u).Error != nil {
		return false
	}
	return u.Password == ""
}

// IsPasswordEmptyByUser 判断密码是否为空，主要用于第三方登录的自动注册
func (us *UserService) IsPasswordEmptyByUser(u *model.User) bool {
	return us.IsPasswordEmptyById(u.Id)
}

// Register 注册, 如果用户名已存在则返回nil
func (us *UserService) Register(username string, email string, password string, status model.StatusCode) *model.User {
	u := &model.User{
		Username: username,
		Email:    email,
		Password: password,
		GroupId:  1,
		Status:   status,
	}
	err := us.Create(u)
	if err != nil {
		return nil
	}
	return u
}

func (us *UserService) TokenList(page uint, size uint, f func(tx *gorm.DB)) *model.UserTokenList {
	res := &model.UserTokenList{}
	res.Page = int64(page)
	res.PageSize = int64(size)
	tx := DB.Model(&model.UserToken{})
	if f != nil {
		f(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, size))
	tx.Find(&res.UserTokens)
	return res
}

func (us *UserService) TokenInfoById(id uint) *model.UserToken {
	ut := &model.UserToken{}
	DB.Where("id = ?", id).First(ut)
	return ut
}

func (us *UserService) DeleteToken(l *model.UserToken) error {
	return DB.Delete(l).Error
}

// Helper functions, used for formatting username
func (us *UserService) formatUsername(username string) string {
	username = strings.ReplaceAll(username, " ", "")
	username = strings.ToLower(username)
	return username
}

// Helper functions, getUserCount
func (us *UserService) getUserCount() int64 {
	var count int64
	DB.Model(&model.User{}).Count(&count)
	return count
}

// helper functions, getAdminUserCount
func (us *UserService) getAdminUserCount() int64 {
	var count int64
	DB.Model(&model.User{}).Where("is_admin = ?", true).Count(&count)
	return count
}

// UserTokenExpireTimestamp 生成用户token过期时间
func (us *UserService) UserTokenExpireTimestamp() int64 {
	exp := Config.App.TokenExpire
	if exp == 0 {
		// 默认两小时。web 后台(BackendUserAuth)不自动续期，到期需重新登录；
		// 客户端(rustauth)仍走滑动续期，活跃会话不受影响。
		exp = 7200
	}
	return time.Now().Add(exp).Unix()
}

func (us *UserService) RefreshAccessToken(ut *model.UserToken) {
	ut.ExpiredAt = us.UserTokenExpireTimestamp()
	DB.Model(ut).Update("expired_at", ut.ExpiredAt)
}

func (us *UserService) AutoRefreshAccessToken(ut *model.UserToken) {
	if ut.ExpiredAt-time.Now().Unix() < Config.App.TokenExpire.Milliseconds()/3000 {
		us.RefreshAccessToken(ut)
	}
}

func (us *UserService) BatchDeleteUserToken(ids []uint) error {
	return DB.Where("id in ?", ids).Delete(&model.UserToken{}).Error
}

func (us *UserService) VerifyJWT(token string) (uint, error) {
	return Jwt.ParseToken(token)
}

// IsUsernameExists 判断用户名是否存在, it will check the internal database and LDAP(if enabled)
func (us *UserService) IsUsernameExists(username string) bool {
	return us.IsUsernameExistsLocal(username) || AllService.LdapService.IsUsernameExists(username)
}

func (us *UserService) IsUsernameExistsLocal(username string) bool {
	u := &model.User{}
	DB.Where("username = ?", username).First(u)
	return u.Id != 0
}

func (us *UserService) IsEmailExistsLdap(email string) bool {
	return AllService.LdapService.IsEmailExists(email)
}

// ===================== MFA (TOTP) =====================

const MfaRecoveryCount = 8

// GenerateMfaRecoveryCodes 生成一次性恢复码
func (us *UserService) GenerateMfaRecoveryCodes() []string {
	codes := make([]string, 0, MfaRecoveryCount)
	for i := 0; i < MfaRecoveryCount; i++ {
		codes = append(codes, utils.RandomString(10))
	}
	return codes
}

// SetMfaSecret 暂存 TOTP 密钥（尚未启用）
func (us *UserService) SetMfaSecret(u *model.User, secret string) error {
	u.MfaSecret = secret
	return DB.Model(u).Update("mfa_secret", secret).Error
}

// EnableMfa 启用 MFA 并生成恢复码，返回恢复码列表
func (us *UserService) EnableMfa(u *model.User) ([]string, error) {
	codes := us.GenerateMfaRecoveryCodes()
	u.MfaEnabled = true
	u.MfaRecovery = strings.Join(codes, ",")
	err := DB.Model(u).Updates(map[string]interface{}{
		"mfa_enabled":  true,
		"mfa_recovery": u.MfaRecovery,
	}).Error
	return codes, err
}

// DisableMfa 关闭 MFA 并清除密钥与恢复码
func (us *UserService) DisableMfa(u *model.User) error {
	u.MfaEnabled = false
	u.MfaSecret = ""
	u.MfaRecovery = ""
	return DB.Model(u).Updates(map[string]interface{}{
		"mfa_enabled":  false,
		"mfa_secret":   "",
		"mfa_recovery": "",
	}).Error
}

// defaultMfaTotpSkew 时钟漂移周期数（每个周期 30s）的内置默认值。
// 当 config.yaml 未配置 mfa_totp_skew 或配置值 <1 时回退到此值（Skew=3 → ±90s）。
// 标准库 totp.Validate 默认 Skew=1（仅容忍 ±30s）。运维中服务器/客户端时钟偏差、
// 或用户输码耗时，偶发会踩线失败；这里默认 Skew=3 作为健壮性加固，且可由 config 调整。
// 注意：若“始终”校验失败，应优先排查 mfa_secret 是否正确落库与时间是否同步，
// 而非无限制放宽窗口掩盖真正的代码/数据问题。
const defaultMfaTotpSkew = 3

// mfaSkew 计算最终生效的 TOTP 校验时钟容差（周期数）。
// 优先使用全局配置 Config.MfaTotpSkew；若配置为空（Config==nil）或配置值 <1（含未配置/0），
// 则回退到 defaultMfaTotpSkew，保证行为健壮且向后兼容。
func (us *UserService) mfaSkew() int {
	if Config != nil && Config.MfaTotpSkew >= 1 {
		return Config.MfaTotpSkew
	}
	return defaultMfaTotpSkew
}

// VerifyMfaCode 校验 TOTP 动态码
func (us *UserService) VerifyMfaCode(u *model.User, code string) bool {
	// secret 为空说明启用流程/Migration 未正确落库，直接拒绝并打告警便于排障，
	// 避免静默返回 false 让“验证码错误”难以定位根因。
	if u.MfaSecret == "" {
		Logger.Warnf("[MFA] VerifyMfaCode: u.Id=%d mfa_secret 为空，跳过校验（疑似启用流程/Migration 未落库）", u.Id)
		return false
	}
	ok, err := totp.ValidateCustom(code, u.MfaSecret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      uint(us.mfaSkew()),
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		Logger.Warnf("[MFA] VerifyMfaCode: u.Id=%d 校验异常: %v", u.Id, err)
		return false
	}
	return ok
}

// VerifyMfaRecovery 校验恢复码并消费（一次性），成功返回 true
func (us *UserService) VerifyMfaRecovery(u *model.User, code string) bool {
	if u.MfaRecovery == "" {
		return false
	}
	codes := strings.Split(u.MfaRecovery, ",")
	for i, c := range codes {
		if c != "" && c == code {
			codes = append(codes[:i], codes[i+1:]...)
			u.MfaRecovery = strings.Join(codes, ",")
			DB.Model(u).Update("mfa_recovery", u.MfaRecovery)
			return true
		}
	}
	return false
}
