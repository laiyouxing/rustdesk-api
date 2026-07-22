package payverify

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// ReplayGuard 回调防重放校验
// 检查回调中的时间戳是否落在 [now - window, now + window] 窗口内
type ReplayGuard struct {
	// windowSec 窗口秒数
	windowSec int
	// timestampField 时间戳字段名（用于错误信息）
	timestampField string
}

// NewReplayGuard 创建防重放守卫
// windowSec: 允许的时间偏差窗口（秒），建议 300（±5 分钟）
func NewReplayGuard(windowSec int, timestampField string) *ReplayGuard {
	return &ReplayGuard{
		windowSec:      windowSec,
		timestampField: timestampField,
	}
}

// Check 校验时间戳是否在允许窗口内
// timestamp: 回调中的时间戳字符串（Unix 秒级）
// 返回 nil 表示通过，否则返回错误描述
func (g *ReplayGuard) Check(timestamp string) error {
	if g.windowSec <= 0 {
		// windowSec <= 0 表示关闭防重放
		return nil
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", g.timestampField, timestamp, err)
	}

	now := time.Now().Unix()
	diff := math.Abs(float64(now - ts))

	if diff > float64(g.windowSec) {
		return fmt.Errorf(
			"replay attack detected: %s=%d, now=%d, window=%ds, diff=%ds",
			g.timestampField, ts, now, g.windowSec, int(diff),
		)
	}
	return nil
}
