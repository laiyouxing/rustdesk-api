package admin

import (
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// parseAuditTime 解析前端传来的时间。
//
// 支持三种写法（按顺序尝试）：
//   - unix 秒（纯数字，10 位）
//   - "2006-01-02 15:04:05"
//   - "2006-01-02"（当天 00:00:00）
//
// 解析失败返回零值 + false，调用方据此忽略该条件（不报错、不静默丢数据）。
func parseAuditTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if len(s) >= 9 && len(s) <= 11 {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			// 10 位为秒，13 位为毫秒；这里只接受秒级以保持语义清晰
			if n > 1000000000 && n < 10000000000 {
				return time.Unix(n, 0), true
			}
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// likeOrEmpty 生成 `column like ?`，值两端加 %；空值不加条件。
func likeIfNotEmpty(tx *gorm.DB, column, value string) {
	if v := strings.TrimSpace(value); v != "" {
		tx.Where(column+" like ?", "%"+v+"%")
	}
}

// ApplyAuditCommon 应用连接日志/文件日志共用的筛选条件。
func ApplyAuditCommon(tx *gorm.DB, q *AuditQuery) {
	likeIfNotEmpty(tx, "peer_id", q.PeerId)
	likeIfNotEmpty(tx, "from_peer", q.FromPeer)
	likeIfNotEmpty(tx, "from_name", q.FromName)
	likeIfNotEmpty(tx, "ip", q.Ip)
	likeIfNotEmpty(tx, "uuid", q.Uuid)

	if q.Type != nil {
		tx.Where("type = ?", *q.Type)
	}

	if q.StartTime != "" {
		if t, ok := parseAuditTime(q.StartTime); ok {
			tx.Where("created_at >= ?", t)
		}
	}
	if q.EndTime != "" {
		if t, ok := parseAuditTime(q.EndTime); ok {
			// 只给日期（没有时分秒）时按当天结束时间算，符合直觉
			if len(strings.TrimSpace(q.EndTime)) <= 10 {
				t = t.Add(24*time.Hour - time.Second)
			}
			tx.Where("created_at <= ?", t)
		}
	}

	// 关键字：多个字段 OR 模糊。
	// 必须自带括号：gorm 把多个 Where 用 AND 串联时不保证给 OR 子句加括号，
	// 否则会变成 `其他条件 AND a like ? OR b like ?` 这种错误分组。
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + kw + "%"
		tx.Where(
			"(peer_id like ? OR from_peer like ? OR from_name like ? OR ip like ? OR uuid like ?)",
			like, like, like, like, like,
		)
	}
}

// ApplyAuditOrder 应用排序。白名单字段，避免拼接注入。
func ApplyAuditOrder(tx *gorm.DB, q *AuditQuery) {
	field := "id"
	switch q.Order {
	case "created_at":
		field = "created_at"
	case "id":
		field = "id"
	}
	dir := "desc"
	if strings.EqualFold(q.OrderDir, "asc") {
		dir = "asc"
	}
	tx.Order(field + " " + dir)
}
