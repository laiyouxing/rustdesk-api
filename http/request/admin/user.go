package admin

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
)

type UserForm struct {
	Id        uint   `json:"id"`
	Username  string `json:"username" validate:"required,gte=2,lte=32"`
	Email     string `json:"email"`
	Nickname  string           `json:"nickname"`
	Avatar    string           `json:"avatar"`
	GroupId   uint             `json:"group_id" validate:"required"`
	IsAdmin   *bool            `json:"is_admin" `
	Status    model.StatusCode `json:"status" validate:"required,gte=0"`
	Remark    string           `json:"remark"`
	ExpiredAt int64            `json:"expired_at"`
}

func (uf *UserForm) FromUser(user *model.User) *UserForm { ... }
func (uf *UserForm) ToUser() *model.User { ... }

type InvitationDeleteForm struct {
	Id uint `json:"id" validate:"required"`
}

type InvitationBatchForm struct {
	Count         int    `json:"count" validate:"required,min=1,max=200"`
	ExpiredAt     int64  `json:"expired_at"`
	UserExpiredAt int64  `json:"user_expired_at"`
	Remark        string `json:"remark"`
}
