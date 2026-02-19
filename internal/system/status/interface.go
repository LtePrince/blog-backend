package status

import "context"

type ISystem interface {
	GetSystemStatus(ctx context.Context) (*SystemStatus, error)
}

func NewSystemStatus(ss *SystemStatus) ISystem {
	if ss == nil {
		ss = &SystemStatus{} // 防止空指针
	}
	return &System{
		status: ss,
	}
}
