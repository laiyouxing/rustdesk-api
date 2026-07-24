package model

type Announcement struct {
	IdModel
	Title   string `json:"title" gorm:"type:varchar(255);not null"`
	Content string `json:"content" gorm:"type:text;not null"`
	Status  int    `json:"status" gorm:"default:1;comment:1=active,0=inactive"`
	TimeModel
}
