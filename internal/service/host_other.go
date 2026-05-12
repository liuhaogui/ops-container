//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package service

// diskUsage 在非 Unix 平台降级到 0，0；container-api 生产部署目标是 Linux，
// 该兜底仅保证 Windows / 其它系统能 go build 跑通本地开发。
func diskUsage(_ string) (uint64, uint64) {
	return 0, 0
}
