//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package service

import (
	"strings"
	"syscall"
)

// diskUsage 取 path 所在挂载点的已用/总容量。失败时返回 (0,0)。
func diskUsage(path string) (uint64, uint64) {
	target := strings.TrimSpace(path)
	if target == "" {
		target = "/"
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(target, &stat); err != nil {
		return 0, 0
	}
	bsize := uint64(stat.Bsize)
	total := uint64(stat.Blocks) * bsize
	free := uint64(stat.Bavail) * bsize
	if total < free {
		return 0, total
	}
	return total - free, total
}
