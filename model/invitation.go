package model

type Invitation struct {
	IdModel
	Code      string `json:"code" gorm:"uniqueIndex;size:128;not null;"`
	MaxUsers  int    `json:"max_users" gorm:"default:0;not null;"`  // 0=无限次使用
	UsedUsers int    `json:"used_users" gorm:"default:0;not null;"`
	ExpiredAt int64  `json:"expired_at" gorm:"default:0;not null;"` // 邀请码过期时间戳，0=永不过期
	Remark    string `json:"remark" gorm:"default:'';not null;"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

type InvitationList struct {
	Invitations []*Invitation `json:"list,omitempty"`
	Pagination
}
