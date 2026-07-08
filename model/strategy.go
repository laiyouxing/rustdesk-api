package model

// Strategy 远程策略配置
// 管理员通过策略下发放置配置给客户端
type Strategy struct {
	IdModel
	Name        string `json:"name" gorm:"type:varchar(100);default:'';not null;comment:'策略名称'"`
	ConfigItems string `json:"config_items" gorm:"type:text;comment:'JSON格式的配置项 key=value 多行'"`
	Priority    int    `json:"priority" gorm:"type:int;default:0;comment:'优先级，数字越大越优先'"`
	Status      int    `json:"status" gorm:"type:tinyint;default:1;comment:'1=启用 2=禁用'"`
	UserId      uint   `json:"user_id" gorm:"default:0;not null;index"`
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

