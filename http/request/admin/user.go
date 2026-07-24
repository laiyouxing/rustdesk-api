package admin

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
)

type UserForm struct {
	Id        uint   `json:"id"`
	Username  string `json:"username" validate:"required,gte=2,lte=32"`
	Email     string `json:"email"` //validate:"required,email" email不强制
	//Password string           `json:"password" validate:"required,gte=4,lte=20"`
	Nickname  string           `json:"nickname"`
	Avatar    string           `json:"avatar"`
	GroupId   uint             `json:"group_id" validate:"required"`
	Role      string           `json:"role"`
	IsAdmin   *bool            `json:"is_admin" `
	Status    model.StatusCode `json:"status" validate:"required,gte=0"`
	Remark    string           `json:"remark"`
	ExpiredAt int64            `json:"expired_at"`
}

func (uf *UserForm) FromUser(user *model.User) *UserForm {
	uf.Id = user.Id
	uf.Username = user.Username
	uf.Nickname = user.Nickname
	uf.Email = user.Email
	uf.Avatar = user.Avatar
	uf.GroupId = user.GroupId
	uf.Role = user.Role
	uf.IsAdmin = user.IsAdmin
	uf.Status = user.Status
	uf.Remark = user.Remark
	uf.ExpiredAt = user.ExpiredAt
	return uf
}
func (uf *UserForm) ToUser() *model.User {
	user := &model.User{}
	user.Id = uf.Id
	user.Username = uf.Username
	user.Nickname = uf.Nickname
	user.Email = uf.Email
	user.Avatar = uf.Avatar
	user.GroupId = uf.GroupId
	user.Role = uf.Role
	user.IsAdmin = uf.IsAdmin
	user.Status = uf.Status
	user.Remark = uf.Remark
	user.ExpiredAt = uf.ExpiredAt
	return user
}

type PageQuery struct {
	Page     uint `form:"page"`
	PageSize uint `form:"page_size"`
}

type UserQuery struct {
	PageQuery
	Username string `form:"username"`
	GroupId  uint   `form:"group_id"`
}
type UserPasswordForm struct {
	Id       uint   `json:"id" validate:"required"`
	Password string `json:"password" validate:"required,gte=4,lte=32"`
}

type ChangeCurPasswordForm struct {
	OldPassword string `json:"old_password" validate:"required,gte=4,lte=32"`
	NewPassword string `json:"new_password" validate:"required,gte=4,lte=32"`
}
type GroupUsersQuery struct {
	IsMy   int  `json:"is_my"`
	UserId uint `json:"user_id"`
}

// MfaEnableForm 启用 MFA 时校验动态码
type MfaEnableForm struct {
	Code string `json:"code" validate:"required" label:"动态码"`
}

// MfaDisableForm 关闭 MFA 时需验证登录密码
type MfaDisableForm struct {
	Password string `json:"password" validate:"required" label:"密码"`
}

// MfaResetForm 管理员强制重置用户 MFA
type MfaResetForm struct {
	UserId uint `json:"user_id" validate:"required" label:"用户ID"`
}

type RegisterForm struct {
	Username        string `json:"username" validate:"required,gte=2,lte=32"`
	Email           string `json:"email"` // validate:"required,email"
	Password        string `json:"password" validate:"required,gte=4,lte=32"`
	ConfirmPassword string `json:"confirm_password" validate:"required,gte=4,lte=32"`
	InviteCode      string `json:"invite_code"`
}

type UserTokenBatchDeleteForm struct {
	Ids []uint `json:"ids" validate:"required"`
}


