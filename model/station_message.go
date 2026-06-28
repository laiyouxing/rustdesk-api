package model

type StationMessage struct {
	RowId     uint   `json:"row_id" gorm:"primaryKey"`
	Type      string `json:"type" gorm:"size:32;not null;default:''"`      // offline/alert/info
	Title     string `json:"title" gorm:"size:200;not null;default:''"`
	Content   string `json:"content" gorm:"type:text"`
	PeerId    string `json:"peer_id" gorm:"size:128;not null;default:''"`
	IsRead    int    `json:"is_read" gorm:"default:0"`                     // 0=unread 1=read
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

type StationMessageList struct {
	Total int64             `json:"total"`
	List  []StationMessage  `json:"list"`
}

func (StationMessage) TableName() string {
	return "station_messages"
}
