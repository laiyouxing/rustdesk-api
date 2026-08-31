//go:build windows

package service

import (
	"golang.org/x/sys/windows"
)

// diskFreePercent 返回 path 所在磁盘的可用空间百分比（0~100）
func diskFreePercent(path string) (float64, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return 0, err
	}
	if totalBytes == 0 {
		return 0, nil
	}
	return float64(totalFreeBytes) / float64(totalBytes) * 100, nil
}
