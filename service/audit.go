package service

import (
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
	"time"
)

type AuditService struct {
}

func (as *AuditService) AuditConnList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AuditConnList) {
	res = &model.AuditConnList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AuditConn{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.AuditConns)
	return
}

// Create 创建
func (as *AuditService) CreateAuditConn(u *model.AuditConn) error {
	res := DB.Create(u).Error
	return res
}
func (as *AuditService) DeleteAuditConn(u *model.AuditConn) error {
	return DB.Delete(u).Error
}

// Update 更新
func (as *AuditService) UpdateAuditConn(u *model.AuditConn) error {
	return DB.Model(u).Updates(u).Error
}

// InfoByPeerIdAndConnId
func (as *AuditService) InfoByPeerIdAndConnId(peerId string, connId int64) (res *model.AuditConn) {
	res = &model.AuditConn{}
	DB.Where("peer_id = ? and conn_id = ?", peerId, connId).First(res)
	return
}

// ConnInfoById
func (as *AuditService) ConnInfoById(id uint) (res *model.AuditConn) {
	res = &model.AuditConn{}
	DB.Where("id = ?", id).First(res)
	return
}

// FileInfoById
func (as *AuditService) FileInfoById(id uint) (res *model.AuditFile) {
	res = &model.AuditFile{}
	DB.Where("id = ?", id).First(res)
	return
}

func (as *AuditService) AuditFileList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AuditFileList) {
	res = &model.AuditFileList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AuditFile{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.AuditFiles)
	return
}

// CreateAuditFile
func (as *AuditService) CreateAuditFile(u *model.AuditFile) error {
	res := DB.Create(u).Error
	return res
}
func (as *AuditService) DeleteAuditFile(u *model.AuditFile) error {
	return DB.Delete(u).Error
}

// Update 更新
func (as *AuditService) UpdateAuditFile(u *model.AuditFile) error {
	return DB.Model(u).Updates(u).Error
}

func (as *AuditService) BatchDeleteAuditConn(ids []uint) error {
	return DB.Where("id in (?)", ids).Delete(&model.AuditConn{}).Error
}

// StartStaleConnCloseSweep 后台定时清理“进行中”的孤儿连接审计记录。
// 触发关闭的条件（满足其一即可）：
//  1. 对端设备已离线超过 5 分钟（崩溃 / 网络中断 / 进程被杀死等场景）；
//  2. 记录创建已超过 24 小时（兜底，覆盖控制端升级替换、未收到 close 事件的场景）。
//
// 避免旧客户端退出时未发送 close 审计，导致首页“最近连接记录”永久卡在“进行中”。
// 该操作幂等，多实例部署也安全。
func (as *AuditService) StartStaleConnCloseSweep() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	as.closeStaleConns()
	for range ticker.C {
		as.closeStaleConns()
	}
}

func (as *AuditService) closeStaleConns() {
	now := time.Now()
	offlineThreshold := now.Unix() - 300      // 对端离线超过 5 分钟
	maxAgeThreshold := now.Add(-24 * time.Hour) // 记录超过 24 小时
	sub := DB.Model(&model.Peer{}).
		Select("peer_id").
		Where("last_online_time <= ? OR last_online_time = 0", offlineThreshold)
	res := DB.Model(&model.AuditConn{}).
		Where("close_time = 0").
		Where("(peer_id IN (?)) OR (created_at < ?)", sub, maxAgeThreshold).
		Update("close_time", now.Unix())
	if res.Error != nil {
		global.Logger.Warn("closeStaleConns", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		global.Logger.Infof("closeStaleConns: closed %d stale 'in-progress' audit_conn records", res.RowsAffected)
	}
}

func (as *AuditService) BatchDeleteAuditFile(ids []uint) error {
	return DB.Where("id in (?)", ids).Delete(&model.AuditFile{}).Error
}
