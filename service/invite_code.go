package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

// InviteCodeService 订阅邀请码服务
type InviteCodeService struct {
}

// NewInviteCodeService 创建 InviteCodeService
func NewInviteCodeService() *InviteCodeService {
	return &InviteCodeService{}
}

// Db 返回 DB 实例
func (s *InviteCodeService) Db() *gorm.DB {
	return DB
}

const base62Charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// generateCode 生成 32 位 base62 随机串
func (s *InviteCodeService) generateCode() string {
	b := make([]byte, 32)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(base62Charset))))
		b[i] = base62Charset[n.Int64()]
	}
	return string(b)
}

// Generate 生成邀请码并绑定用户（自动发码流程）
// 如果 orderID 不为空，会在同一事务中绑定
func (s *InviteCodeService) Generate(plan string, userID uint, boundOrderID string, expireDays int) (*model.InviteCode, error) {
	return s.GenerateWithDB(s.Db(), plan, userID, boundOrderID, expireDays)
}

// GenerateWithDB 在指定 DB 连接（可以是事务 tx）上生成邀请码。
// 在事务回调中必须传 tx，否则 SQLite 会因数据库级排他锁导致 "database is locked" 死锁。
func (s *InviteCodeService) GenerateWithDB(db *gorm.DB, plan string, userID uint, boundOrderID string, expireDays int) (*model.InviteCode, error) {
	// 永久码：码本身过期时间设为 9999 年
	if plan == "forever" || expireDays >= 9999 {
		expireDays = 99999
	}
	// 重试最多 10 次以避免唯一索引冲突
	var code *model.InviteCode
	for i := 0; i < 10; i++ {
		codeStr := s.generateCode()
		code = &model.InviteCode{
			Code:         codeStr,
			Plan:         plan,
			ExpireDays:   expireDays,
			ExpireAt:     expireAtFor(expireDays),
			Status:       "unused",
			BoundOrderID: boundOrderID,
		}
		err := db.Create(code).Error
		if err == nil {
			return code, nil
		}
		// 唯一索引冲突则重试
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "duplicate") {
			continue
		}
		return nil, fmt.Errorf("generate invite code: %w", err)
	}
	return nil, fmt.Errorf("generate invite code: failed after 10 retries")
}

// expireAtFor 计算授权码的码本身过期时间；永久码返回 9999 年
func expireAtFor(expireDays int) time.Time {
	if expireDays >= 9999 {
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Now().AddDate(0, 0, expireDays)
}

// Activate 使用邀请码激活用户订阅（顺延策略）
// 事务：更新 code 状态 + 更新 user.subscription_expire_at
func (s *InviteCodeService) Activate(codeStr string, userID uint) (*model.InviteCode, error) {
	ic := &model.InviteCode{}
	if err := s.Db().Where("code = ?", codeStr).First(ic).Error; err != nil {
		return nil, fmt.Errorf("code not found: %w", err)
	}

	// 状态校验
	switch ic.Status {
	case "used":
		return nil, fmt.Errorf("code already used")
	case "revoked":
		return nil, fmt.Errorf("code revoked")
	default:
		// unused
	}

	// 有效期校验
	if ic.ExpireAt.Before(time.Now()) {
		return nil, fmt.Errorf("code expired")
	}

	now := time.Now()
	err := s.Db().Transaction(func(tx *gorm.DB) error {
		// 原子更新码状态（乐观锁）
		res := tx.Model(&model.InviteCode{}).
			Where("id = ? AND status = ?", ic.Id, "unused").
			Updates(map[string]interface{}{
				"status":   "used",
				"used_by":  userID,
				"used_at":  &now,
				"expire_at": ic.ExpireAt,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("code already used or revoked (concurrent)")
		}

		// 更新用户订阅过期时间（顺延策略）
		user := &model.User{}
		if err := tx.Where("id = ?", userID).First(user).Error; err != nil {
			return err
		}
		var newExpire time.Time
		if ic.IsForever() {
			// 永久授权码：会员设为 9999 年
			newExpire = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		} else {
			periodDuration := time.Duration(ic.ExpireDays*24) * time.Hour
			if user.SubscriptionExpireAt == nil || user.SubscriptionExpireAt.Before(now) {
				newExpire = now.Add(periodDuration)
			} else {
				newExpire = user.SubscriptionExpireAt.Add(periodDuration)
			}
		}
		// 只更新订阅时长与过期日期；expired_at 保持原值（0=永不过期），
		// 保证订阅到期后用户仍可登录后台续费。
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			Updates(map[string]interface{}{
				"subscription_plan":      ic.Plan,
				"subscription_expire_at": &newExpire,
			}).Error; err != nil {
			return err
		}

		ic.Status = "used"
		ic.UsedBy = userID
		ic.UsedAt = &now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ic, nil
}

// Revoke 失效邀请码（仅管理员）
func (s *InviteCodeService) Revoke(id uint) error {
	now := time.Now()
	res := s.Db().Model(&model.InviteCode{}).
		Where("id = ? AND status = ?", id, "unused").
		Updates(map[string]interface{}{
			"status":     "revoked",
			"revoked_at": &now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("code not found or already used/revoked")
	}
	return nil
}

// List 分页查询邀请码列表
// 支持按 status / plan / used_by 过滤
type InviteCodeFilter struct {
	Status  string
	Plan    string
	UsedBy  uint
	Page    int
	PageSize int
}

func (s *InviteCodeService) List(filter InviteCodeFilter) ([]*model.InviteCode, int64, error) {
	query := s.Db().Model(&model.InviteCode{})

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Plan != "" {
		query = query.Where("plan = ?", filter.Plan)
	}
	if filter.UsedBy > 0 {
		query = query.Where("used_by = ?", filter.UsedBy)
	}

	var total int64
	query.Count(&total)

	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}

	var list []*model.InviteCode
	query.Order("id desc").
		Offset((filter.Page - 1) * filter.PageSize).
		Limit(filter.PageSize).
		Find(&list)

	return list, total, nil
}

// InfoByCode 根据 code 查询
func (s *InviteCodeService) InfoByCode(code string) *model.InviteCode {
	ic := &model.InviteCode{}
	s.Db().Where("code = ?", code).First(ic)
	return ic
}

// InfoByOrderID 根据订单号查询已生成的邀请码
func (s *InviteCodeService) InfoByOrderID(orderID string) *model.InviteCode {
	ic := &model.InviteCode{}
	s.Db().Where("bound_order_id = ?", orderID).First(ic)
	return ic
}
