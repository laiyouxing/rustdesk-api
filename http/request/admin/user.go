...existing InvitationDeleteForm...

type InvitationBatchForm struct {
	Count         int    `json:"count" validate:"required,min=1,max=200"`
	ExpiredAt     int64  `json:"expired_at"`
	UserExpiredAt int64  `json:"user_expired_at"`
	Remark        string `json:"remark"`
}