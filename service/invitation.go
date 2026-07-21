package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type InvitationService struct {
}

func NewInvitationService() *InvitationService {
	return &InvitationService{}
}

func (is *InvitationService) Db() *gorm.DB {
	return global.DB
}

// generateCode 生成6位随机邀请码（数字+大小写英文）
func (is *InvitationService) generateCode() string {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 32)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// Create 创建邀请码
func (is *InvitationService) Create(invitation *model.Invitation) error {
	if invitation.Code == "" {
		invitation.Code = is.generateCode()
	}
	invitation.CreatedAt = time.Now().Unix()
	return is.Db().Create(invitation).Error
}

// List 分页查询邀请码列表
func (is *InvitationService) List(page, pageSize int64) ([]*model.Invitation, int64) {
	var list []*model.Invitation
	var total int64
	query := is.Db().Model(&model.Invitation{})
	query.Count(&total)
	query.Order("id desc").Offset(int((page - 1) * pageSize)).Limit(int(pageSize)).Find(&list)
	return list, total
}

// InfoById 根据ID查询
func (is *InvitationService) InfoById(id uint) *model.Invitation {
	inv := &model.Invitation{}
	is.Db().Where("id = ?", id).First(inv)
	return inv
}

// InfoByCode 根据邀请码查询
func (is *InvitationService) InfoByCode(code string) *model.Invitation {
	inv := &model.Invitation{}
	is.Db().Where("code = ?", code).First(inv)
	return inv
}

// Validate 验证邀请码是否可用
func (is *InvitationService) Validate(code string) bool {
	inv := is.InfoByCode(code)
	if inv.Id == 0 {
		return false
	}
	// 检查是否过期
	if inv.ExpiredAt > 0 && inv.ExpiredAt < time.Now().Unix() {
		return false
	}
	// 检查使用次数是否已达上限
	if inv.MaxUsers > 0 && inv.UsedUsers >= inv.MaxUsers {
		return false
	}
	return true
}

// Use 使用邀请码（原子操作，防止并发重复使用）
func (is *InvitationService) Use(code string) error {
	result := is.Db().Model(&model.Invitation{}).
		Where("code = ? AND used_users = 0 AND (expired_at = 0 OR expired_at > ?)", code, time.Now().Unix()).
		UpdateColumn("used_users", 1)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("invitation code invalid or already used")
	}
	return nil
}

// Delete 删除邀请码
func (is *InvitationService) Delete(id uint) error {
	return is.Db().Delete(&model.Invitation{}, id).Error
}
