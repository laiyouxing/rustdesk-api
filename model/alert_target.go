package model

type AlertTarget struct {
	RowId      uint   `json:"row_id" gorm:"primaryKey"`
	AlertId    uint   `json:"alert_id" gorm:"not null;default:0;index"` // FK to AlertConfig
	TargetType string `json:"target_type" gorm:"size:16;not null;default:''"` // collection / peer
	TargetId   string `json:"target_id" gorm:"size:128;not null;default:''"`  // collection_id or peer_id
	TargetName string `json:"target_name" gorm:"size:200;not null;default:''"` // display name
}

func (AlertTarget) TableName() string {
	return "alert_targets"
}
