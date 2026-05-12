package service

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// HostStats 与 ops-api `service.HostStats` 对齐：字段名/单位严格一致，方便 ops-api 直接反序列化。
type HostStats struct {
	Hostname       string  `json:"hostname"`
	Version        string  `json:"version"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryUsed     uint64  `json:"memory_used"`
	MemoryTotal    uint64  `json:"memory_total"`
	DiskUsed       uint64  `json:"disk_used"`
	DiskTotal      uint64  `json:"disk_total"`
	ContainerCount int     `json:"container_count"`
}

// HostService 采集主机 + docker daemon 维度的运行时指标。
// 仅依赖标准库 + docker client，不引入 gopsutil 等新依赖；Linux 走 /proc，
// 其它系统降级到 0（container-api 生产部署目标是 Linux，dev 跨平台容忍降级）。
type HostService struct {
	client  *client.Client
	version string
}

func NewHostService(cli *client.Client, version string) *HostService {
	return &HostService{client: cli, version: version}
}

// Stats 一次采样返回主机健康摘要。CPU 通过 100ms 间隔的两次 /proc/stat 采样估算；
// docker 数据目录从 ContainerService 的 client.Info 拿，避免硬编码 /var/lib/docker。
func (h *HostService) Stats(ctx context.Context) HostStats {
	stats := HostStats{Version: h.version}
	if hostname, err := os.Hostname(); err == nil {
		stats.Hostname = hostname
	}

	if h.client != nil {
		if info, err := h.client.Info(ctx); err == nil {
			stats.DiskUsed, stats.DiskTotal = diskUsage(info.DockerRootDir)
		}
		if list, err := h.client.ContainerList(ctx, containertypes.ListOptions{All: false}); err == nil {
			stats.ContainerCount = len(list)
		}
	}

	stats.MemoryUsed, stats.MemoryTotal = readMeminfo()
	stats.CPUPercent = sampleCPU(ctx)
	return stats
}

// readMeminfo 解析 /proc/meminfo 返回 (used, total) byte。
// 非 Linux 平台或读取失败时返回 (0,0)，ops-api 端按 0 处理。
func readMeminfo() (uint64, uint64) {
	if runtime.GOOS != "linux" {
		return 0, 0
	}
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var memTotal, memAvailable uint64
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "MemTotal:":
			memTotal = parseKB(parts[1])
		case "MemAvailable:":
			memAvailable = parseKB(parts[1])
		}
	}
	if memTotal == 0 {
		return 0, 0
	}
	used := uint64(0)
	if memTotal > memAvailable {
		used = memTotal - memAvailable
	}
	return used, memTotal
}

func parseKB(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v * 1024
}

// sampleCPU 计算最近 100ms 的总体 CPU 使用率（0~100）。/proc/stat 在 Linux 才有；
// 其它系统降级到 0。误差容忍：本接口的目的是给运维一眼可见的负载指示，不是精确监控。
func sampleCPU(ctx context.Context) float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	a, ok := readProcStat()
	if !ok {
		return 0
	}
	select {
	case <-ctx.Done():
		return 0
	case <-time.After(100 * time.Millisecond):
	}
	b, ok := readProcStat()
	if !ok {
		return 0
	}
	totalDelta := float64(b.total - a.total)
	if totalDelta <= 0 {
		return 0
	}
	idleDelta := float64(b.idle - a.idle)
	usage := (totalDelta - idleDelta) * 100.0 / totalDelta
	if usage < 0 {
		return 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage
}

type cpuSnapshot struct {
	total uint64
	idle  uint64
}

func readProcStat() (cpuSnapshot, bool) {
	raw, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuSnapshot{}, false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return cpuSnapshot{}, false
		}
		var total uint64
		var idle uint64
		for i, v := range fields[1:] {
			parsed, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return cpuSnapshot{}, false
			}
			total += parsed
			// /proc/stat cpu 行字段顺序：user nice system idle iowait irq softirq steal ...
			// 把 idle + iowait 都计入 idle，与 top/htop 口径一致。
			if i == 3 || i == 4 {
				idle += parsed
			}
		}
		return cpuSnapshot{total: total, idle: idle}, true
	}
	return cpuSnapshot{}, false
}
