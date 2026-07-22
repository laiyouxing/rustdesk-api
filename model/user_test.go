package model

import (
	"testing"
	"time"
)

// TestSubscriptionStatus_None 测试从未订阅状态
func TestSubscriptionStatus_None(t *testing.T) {
	u := &User{}
	if status := u.SubscriptionStatus(); status != "none" {
		t.Fatalf("expected 'none', got '%s'", status)
	}
	if u.IsSubscriptionActive() {
		t.Fatal("IsSubscriptionActive should be false for nil expire")
	}
	if days := u.SubscriptionDaysLeft(); days != 0 {
		t.Fatalf("expected 0 days left, got %d", days)
	}
}

// TestSubscriptionStatus_Active 测试订阅有效状态
func TestSubscriptionStatus_Active(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour)
	u := &User{
		SubscriptionPlan:     "pro",
		SubscriptionExpireAt: &future,
	}

	if status := u.SubscriptionStatus(); status != "active" {
		t.Fatalf("expected 'active', got '%s'", status)
	}
	if !u.IsSubscriptionActive() {
		t.Fatal("IsSubscriptionActive should be true")
	}
	if days := u.SubscriptionDaysLeft(); days <= 0 || days > 31 {
		t.Fatalf("expected days between 1-31, got %d", days)
	}
}

// TestSubscriptionStatus_Expired 测试订阅过期状态
func TestSubscriptionStatus_Expired(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	u := &User{
		SubscriptionPlan:     "pro",
		SubscriptionExpireAt: &past,
	}

	if status := u.SubscriptionStatus(); status != "expired" {
		t.Fatalf("expected 'expired', got '%s'", status)
	}
	if u.IsSubscriptionActive() {
		t.Fatal("IsSubscriptionActive should be false for expired")
	}
	if days := u.SubscriptionDaysLeft(); days != 0 {
		t.Fatalf("expected 0 days left for expired, got %d", days)
	}
}

// TestSubscriptionDaysLeft 测试剩余天数计算
func TestSubscriptionDaysLeft(t *testing.T) {
	// 刚好 7 天后过期
	future := time.Now().Add(7 * 24 * time.Hour)
	u := &User{SubscriptionExpireAt: &future}
	days := u.SubscriptionDaysLeft()
	if days < 6 || days > 8 {
		t.Fatalf("expected ~7 days, got %d", days)
	}

	// 刚好 1 天后过期
	future2 := time.Now().Add(24 * time.Hour)
	u2 := &User{SubscriptionExpireAt: &future2}
	days2 := u2.SubscriptionDaysLeft()
	if days2 < 0 || days2 > 2 {
		t.Fatalf("expected ~1 day, got %d", days2)
	}
}

// TestSubscriptionStatus_ExactlyNow 测试正好现在过期
func TestSubscriptionStatus_ExactlyNow(t *testing.T) {
	// 使用微秒级精度
	justNow := time.Now()
	u := &User{SubscriptionExpireAt: &justNow}

	// 由于 Before 在 time.Now() 上是不确定的，我们只检查不会 panic
	_ = u.SubscriptionStatus()
	_ = u.IsSubscriptionActive()
	_ = u.SubscriptionDaysLeft()
}
