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

func (is *InvitationService) generateCode() string {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 32)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func (is *InvitationService) Create(invitation *model.Invitation) error {
	if invitation.Code == "" {
		invitation.Code = is.generateCode()
	}
	invitation.CreatedAt = time.Now().Unix()
	return is.Db().Create(invitation).Error
}

func (is *InvitationService) List(page, pageSize int64) ([]*model.Invitation, int64) {
	var list []*model.Invitation
	var total int64
	query := is.Db().Model(&model.Invitation{})
	query.Count(&total)
	query.Order("id desc").Offset(int((page - 1) * pageSize)).Limit(int(pageSize)).Find(&list)
	return list, total
}

func (is *InvitationService) InfoById(id uint) *model.Invitation {
	inv := &model.Invitation{}
	is.Db().Where("id = ?", id).First(inv)
	return inv
}

func (is *InvitationService) InfoByCode(code string) *model.Invitation {
	inv := &model.Invitation{}
	is.Db().Where("code = ?", code).First(inv)
	return inv
}

func (is *InvitationService) Validate(code string) bool {
	inv := is.InfoByCode(code)
	if inv.Id == 0 {
		return false
	}
	if inv.ExpiredAt > 0 && inv.ExpiredAt < time.Now().Unix() {
		return false
	}
	if inv.MaxUsers > 0 && inv.UsedUsers >= inv.MaxUsers {
		return false
	}
	return true
}

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

func (is *InvitationService) BatchCreate(count int, expiredAt, userExpiredAt int64, remark string) ([]*model.Invitation, error) {
	now := time.Now().Unix()
	expired := expiredAt
	if expired == 0 {
		expired = now + 86400
	}
	var invitations []*model.Invitation
	for i := 0; i < count; i++ {
		inv := &model.Invitation{
			Code:          is.generateCode(),
			MaxUsers:      1,
			ExpiredAt:     expired,
			UserExpiredAt: userExpiredAt,
			Remark:        remark,
			CreatedAt:     now,
		}
		invitations = append(invitations, inv)
	}
	if err := is.Db().Create(invitations).Error; err != nil {
		return nil, err
	}
	return invitations, nil
}

func (is *InvitationService) Delete(id uint) error {
	return is.Db().Delete(&model.Invitation{}, id).Error
}
