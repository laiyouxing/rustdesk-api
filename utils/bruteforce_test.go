package utils

import (
	"testing"
	"time"
)

// TestBruteForce_BanThresholdReached 验证基于内存 per-IP 计数器的封禁阈值逻辑：
// 连续失败达到阈值后该 IP 被封禁，封禁状态可通过 CheckSecurityStatus 读取。
// 该机制即“暴力破解 IP 封禁”的核心（http/middleware/limiter.go 调用本 LoginLimiter）。
func TestBruteForce_BanThresholdReached(t *testing.T) {
	threshold := 3
	ll := NewLoginLimiter(SecurityPolicy{
		BanThreshold: threshold,
		BanDuration:  5 * time.Minute,
	})
	ip := "203.0.113.5"

	// 未达到阈值前不应被封禁
	for i := 0; i < threshold-1; i++ {
		ll.RecordFailedAttempt(ip)
		if banned, _ := ll.CheckSecurityStatus(ip); banned {
			t.Fatalf("IP should not be banned before reaching threshold (attempt %d)", i+1)
		}
	}

	// 达到阈值 -> 应被封禁
	ll.RecordFailedAttempt(ip)
	if banned, _ := ll.CheckSecurityStatus(ip); !banned {
		t.Fatalf("IP should be banned after %d failed attempts", threshold)
	}

	// 不同 IP 互不影响（per-IP 内存 map）
	if banned, _ := ll.CheckSecurityStatus("198.51.100.7"); banned {
		t.Fatalf("different IP should not be affected by another IP's ban")
	}

	// BanThreshold <= 0 时禁用封禁：永远不会被封禁
	disabled := NewLoginLimiter(SecurityPolicy{BanThreshold: 0, CaptchaThreshold: -1})
	for i := 0; i < 10; i++ {
		disabled.RecordFailedAttempt(ip)
	}
	if banned, _ := disabled.CheckSecurityStatus(ip); banned {
		t.Fatalf("with BanThreshold=0 and CaptchaThreshold<0, IP must not be banned (default off)")
	}
}
