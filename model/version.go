package model

type Version struct {
	IdModel
	Version   string `json:"version" gorm:"type:varchar(32);default:'';not null;"`
	Platform  string `json:"platform" gorm:"type:varchar(16);default:'';not null;comment:'windows/macos/linux/ubuntu'"`
	Url       string `json:"url" gorm:"type:varchar(512);default:'';not null;comment:'download URL'"`
	Note      string `json:"note" gorm:"type:text;comment:'release notes'"`
	Status    int    `json:"status" gorm:"type:tinyint;default:1;comment:'1=enable 2=disable'"`
	TimeModel
}
