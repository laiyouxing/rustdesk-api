package api

// ProcessItem 单个监控项的上报状态
type ProcessItem struct {
	Name    string `json:"name"`    // 展示名
	Type    string `json:"type"`    // process | port
	Target  string `json:"target"`  // 进程名 / 端口
	Running bool   `json:"running"` // 是否运行中
}

// ProcessStatusForm 客户端上报的监控状态
type ProcessStatusForm struct {
	PeerId string        `json:"peer_id"`
	Uuid   string        `json:"uuid"`
	Items  []ProcessItem `json:"items"`
}

// ProcessConfigQuery 客户端拉取自己的监控配置
type ProcessConfigQuery struct {
	PeerId string `json:"peer_id" form:"peer_id"`
}

// ProcessRuleForm 后台配置监控规则
type ProcessRuleForm struct {
	RowId        uint   `json:"row_id"`
	PeerId       string `json:"peer_id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Target       string `json:"target"`
	Interval     int    `json:"interval"`
	DownThreshold int   `json:"down_threshold"`
	AlertConfigId uint  `json:"alert_config_id"`
	Enabled      int    `json:"enabled"`
}
