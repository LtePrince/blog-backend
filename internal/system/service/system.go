package service

import (
	"blog-backend/internal/system/status"
	"context"
)

type SystemService struct {
	sys status.ISystem
}

func NewSystemService(sys status.ISystem) *SystemService {
	return &SystemService{
		sys: sys,
	}
}

type GetSystemStatusRequest struct{}

type GetSystemStatusResponse struct {
	Status *status.SystemStatus `json:"status"`
}

func (s *SystemService) GetSystemStatus(
	ctx context.Context,
	req *GetSystemStatusRequest,
) (*GetSystemStatusResponse, error) {
	sysStatus, err := s.sys.GetSystemStatus(ctx)
	if err != nil {
		return nil, err
	}

	return &GetSystemStatusResponse{
		Status: sysStatus,
	}, nil
}
