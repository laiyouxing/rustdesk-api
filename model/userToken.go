package model

type UserToken struct {
	IdModel
	UserId     uint   `json:"user_id" gorm:"default:0;not null;index"`
	DeviceUuid string `json:"device_uuid" gorm:"default:'';omitempty;"`
	DeviceId   string `json:"device_id" gorm:"default:'';omitempty;"`
	Token      string `json:"token" gorm:"default:'';not null;index"`
	ExpiredAt  int64  `json:"expired_at" gorm:"default:0;not null;"`
	// Fingerprint 绑定 token 签发时的客户端特征（IP+User-Agent 的哈希），
	// 用于后端校验请求来源，防止 token 被盗用后从其他环境冒用。
	Fingerprint string `json:"fingerprint" gorm:"default:'';not null;"`
	TimeModel
}

type UserTokenList struct {
	UserTokens []UserToken `json:"list"`
	Pagination
}
