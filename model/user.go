package model

import "time"

type User struct {
	IdModel
	Username string `json:"username" gorm:"default:'';not null;uniqueIndex"`
	Email    string `json:"email" gorm:"default:'';not null;index"`
	// Email	string     	`json:"email" `
	Password string     `json:"-" gorm:"default:'';not null;"`
	Nickname string     `json:"nickname" gorm:"default:'';not null;"`
	Avatar   string     `json:"avatar" gorm:"default:'';not null;"`
	GroupId  uint       `json:"group_id" gorm:"default:0;not null;index"`
	IsAdmin  *bool      `json:"is_admin" gorm:"default:0;not null;"`
	Status   StatusCode `json:"status" gorm:"default:1;not null;"`
	Remark   string     `json:"remark" gorm:"default:'';not null;"`
	ExpiredAt int64     `json:"expired_at" gorm:"default:0;not null;"` // 账户过期时间戳，0=永不过期
	// MFA(TOTP) 相关字段：mfa_secret/mfa_recovery 不对外暴露
	MfaEnabled  bool   `json:"mfa_enabled" gorm:"default:0;not null;"`
	MfaSecret   string `json:"-" gorm:"default:'';not null;"`
	MfaRecovery string `json:"-" gorm:"default:'';not null;"`
	// SubscriptionPlan 订阅套餐标识，空字符串表示未订阅
	SubscriptionPlan string `json:"subscription_plan" gorm:"size:32;default:''"`
	// SubscriptionExpireAt 订阅过期时间，为空表示从未订阅过
	SubscriptionExpireAt *time.Time `json:"subscription_expire_at" gorm:"default:null"`
	TimeModel
}

// SubscriptionStatus 返回订阅状态：active / expired / none
func (u *User) SubscriptionStatus() string {
	if u.SubscriptionExpireAt == nil {
		return "none"
	}
	if u.SubscriptionExpireAt.Before(time.Now()) {
		return "expired"
	}
	return "active"
}

// IsSubscriptionActive 订阅是否有效（过期 = false，从未订阅 = false）
func (u *User) IsSubscriptionActive() bool {
	return u.SubscriptionStatus() == "active"
}

// SubscriptionDaysLeft 返回订阅剩余天数
func (u *User) SubscriptionDaysLeft() int {
	if u.SubscriptionExpireAt == nil {
		return 0
	}
	now := time.Now()
	if u.SubscriptionExpireAt.Before(now) {
		return 0
	}
	days := int(u.SubscriptionExpireAt.Sub(now).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// BeforeSave 钩子用于确保 email 字段有合理的默认值
//func (u *User) BeforeSave(tx *gorm.DB) (err error) {
//	// 如果 email 为空，设置为默认值
//	if u.Email == "" {
//		u.Email = fmt.Sprintf("%s@example.com", u.Username)
//	}
//	return nil
//}

type UserList struct {
	Users []*User `json:"list,omitempty"`
	Pagination
}

var UserRouteNames = []string{
	"MyTagList", "MyAddressBookList", "MyInfo", "MyAddressBookCollection", "MyPeer", "MyShareRecordList", "MyLoginLog",
	"StationMessages", "HomePage", "MySubscription",
}
var AdminRouteNames = []string{"*"}
