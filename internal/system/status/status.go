package status

import (
	"context"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

type System struct {
	status *SystemStatus
}

type SystemStatus struct {
	CPUUsage    float64 `json:"cpu"`
	MemoryUsage float64 `json:"memory"`
	DiskUsage   float64 `json:"disk"`
}

func (s *System) GetSystemStatus(ctx context.Context) (*SystemStatus, error) {
	cpuPercents, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil || len(cpuPercents) == 0 {
		return nil, err
	}
	memStat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, err
	}
	diskStat, err := disk.UsageWithContext(ctx, "/")
	if err != nil {
		return nil, err
	}
	status := &SystemStatus{
		CPUUsage:    float64(int(cpuPercents[0]*100)) / 100,
		MemoryUsage: float64(int(memStat.UsedPercent*100)) / 100,
		DiskUsage:   float64(int(diskStat.UsedPercent*100)) / 100,
	}
	return status, nil
}
