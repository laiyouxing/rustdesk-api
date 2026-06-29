package model

type StunServer struct {
	RowId   uint   `json:"row_id" gorm:"primaryKey"`
	Host    string `json:"host" gorm:"size:200;not null;default:''"`
	Port    int    `json:"port" gorm:"default:3478"`
	Enabled int    `json:"enabled" gorm:"default:1"` // 1=enabled 2=disabled
	Sort    int    `json:"sort" gorm:"default:0"`    // priority order
	Remark  string `json:"remark" gorm:"size:200;not null;default:''"`
}

func (StunServer) TableName() string {
	return "stun_servers"
}
