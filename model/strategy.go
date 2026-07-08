package model

// Strategy 远程策略配置
// bind_type: user=绑定用户, group=绑定设备分组, tag=绑定标签, global=全局
type Strategy struct {
	IdModel
	Name        string `json:"name" gorm:"type:varchar(100);default:'';not null;comment:'策略名称'"`
	ConfigItems string `json:"config_items" gorm:"type:text;comment:'key=value 多行配置'"`
	Priority    int    `json:"priority" gorm:"type:int;default:0;comment:'优先级，数字越大越优先'"`
	Status      int    `json:"status" gorm:"type:tinyint;default:1;comment:'1=启用 2=禁用'"`
	BindType    string `json:"bind_type" gorm:"type:varchar(16);default:'global';comment:'user/group/tag/global'"`
	BindId      uint   `json:"bind_id" gorm:"default:0;not null;index;comment:'策略绑定的用户/设备分组/标签ID, global时为0'"`
	TimeModel
}

func (Strategy) TableName() string {
	return "strategies"
}

type StrategyList struct {
	Strategies []*Strategy `json:"list"`
	Pagination
}

// StrategyOptions 心跳下发的策略配置，与客户端 StrategyOptions 对应
type StrategyOptions struct {
	ConfigOptions map[string]string `json:"config_options"`
	Extra         map[string]string `json:"extra,omitempty"`
}

