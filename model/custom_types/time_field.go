package custom_types

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// TimeField 可空时间字段，用于 GORM 中 *TimeField 指针形式的可空时间戳。
// 内部存储为 Unix 时间戳（秒），JSON 序列化为 ISO8601 格式。
type TimeField time.Time

// Scan 实现 sql.Scanner 接口
func (t *TimeField) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	var tt time.Time
	switch v := src.(type) {
	case time.Time:
		tt = v
	case []byte:
		parsed, err := time.Parse("2006-01-02 15:04:05", string(v))
		if err != nil {
			return err
		}
		tt = parsed
	case string:
		parsed, err := time.Parse("2006-01-02 15:04:05", v)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, v)
			if err != nil {
				return err
			}
		}
		tt = parsed
	case int64:
		tt = time.Unix(v, 0)
	default:
		return fmt.Errorf("cannot scan TimeField from %T", src)
	}
	*t = TimeField(tt)
	return nil
}

// Value 实现 driver.Valuer 接口
func (t TimeField) Value() (driver.Value, error) {
	return time.Time(t), nil
}

// MarshalJSON 实现 json.Marshaler
func (t TimeField) MarshalJSON() ([]byte, error) {
	tt := time.Time(t)
	if tt.IsZero() {
		return []byte("null"), nil
	}
	return tt.MarshalJSON()
}

// UnmarshalJSON 实现 json.Unmarshaler
func (t *TimeField) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var tt time.Time
	if err := tt.UnmarshalJSON(data); err != nil {
		return err
	}
	*t = TimeField(tt)
	return nil
}

// Time 返回 time.Time
func (t TimeField) Time() time.Time {
	return time.Time(t)
}
