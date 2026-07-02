package model

// ClientDownload 客户端下载链接
type ClientDownload struct {
	IdModel
	Platform  string `json:"platform" gorm:"type:varchar(32);default:'';not null;comment:'平台标识'"`
	Name      string `json:"name" gorm:"type:varchar(128);default:'';not null;comment:'显示名称'"`
	Url       string `json:"url" gorm:"type:varchar(512);default:'';not null;comment:'下载链接'"`
	SortOrder int    `json:"sort_order" gorm:"type:int;default:0;comment:'排序'"`
	Status    int    `json:"status" gorm:"type:tinyint;default:1;comment:'1=启用 2=禁用'"`
	TimeModel
}

func (ClientDownload) TableName() string {
	return "client_downloads"
}
