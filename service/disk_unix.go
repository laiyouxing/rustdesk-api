//go:build !windows

package service

import (
	"golang.org/x/sys/unix"
)

// diskFreePercent 返回 path 所在文件系统的可用空间百分比（0~100）
func diskFreePercent(path string) (float64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	if total == 0 {
		return 0, nil
	}
	return float64(free) / float64(total) * 100, nil
}
