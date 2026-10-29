package admin

// PunchDirectOkQuery 直连上报记录查询参数
type PunchDirectOkQuery struct {
	PeerId   string `form:"peer_id"`
	Target   string `form:"target"`
	ConnType string `form:"conn_type"` // direct_v4 | direct_v6 | relay
	PageQuery
}

// PunchDirectOkLogIds 批量删除用
type PunchDirectOkLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}
