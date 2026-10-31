package admin

type AuditQuery struct {
	// —— 字段筛选 ——
	PeerId   string `form:"peer_id"`   // 目标设备 ID
	FromPeer string `form:"from_peer"` // 来源设备 ID
	FromName string `form:"from_name"` // 来源设备名
	Ip       string `form:"ip"`        // 来源 IP
	Uuid     string `form:"uuid"`      // 设备 uuid
	Path     string `form:"path"`      // 文件日志：路径/文件名

	// —— 枚举筛选（用指针区分"未传"与"传了 0"）——
	Type   *int  `form:"type"`    // 0=普通 1=文件传输
	IsFile *bool `form:"is_file"` // 文件日志：是否文件
	// 连接日志的 action：new / close。留空时默认只取 new（保持原行为）。
	Action string `form:"action"`

	// —— 时间范围 ——
	// 两者都支持 "2006-01-02 15:04:05"、"2006-01-02" 或 unix 秒
	StartTime string `form:"start_time"`
	EndTime   string `form:"end_time"`

	// —— 关键字：在 peer_id / from_peer / from_name / ip / uuid 上 OR 模糊 ——
	Keyword string `form:"keyword"`

	// —— 排序：默认 id desc；order 支持 id / created_at ——
	Order    string `form:"order"`
	OrderDir string `form:"order_dir"` // asc / desc

	PageQuery
}

type AuditConnLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}
type AuditFileLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}
