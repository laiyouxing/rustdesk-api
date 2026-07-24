package admin

import "github.com/lejianwen/rustdesk-api/v2/model"

type LoginPayload struct {
	Username      string   `json:"username"`
	Email         string   `json:"email"`
	Avatar        string   `json:"avatar"`
	Token         string   `json:"token"`
	RouteNames    []string `json:"route_names"`
	Nickname      string   `json:"nickname"`
	MfaEnabled    bool     `json:"mfa_enabled"`
	AccountExpired bool    `json:"account_expired"`
	Role          string   `json:"role"`
}

func (lp *LoginPayload) FromUser(user *model.User) {
	lp.Username = user.Username
	lp.Email = user.Email
	lp.Avatar = user.Avatar
	lp.Nickname = user.Nickname
	lp.MfaEnabled = user.MfaEnabled
	role := "user"
	if user.Role != "" {
		role = user.Role
	} else if user.IsAdmin != nil && *user.IsAdmin {
		role = "admin"
	}
	lp.Role = role
}

type UserOauthItem struct {
	Op     string `json:"op"`
	Status int    `json:"status"`
}
