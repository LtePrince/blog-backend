package system

import (
	"blog-backend/internal/system/service"
	"blog-backend/internal/system/status"

	"go.uber.org/fx"
)

var Module = fx.Module("system",
	fx.Provide(
		func() *status.SystemStatus {
			return &status.SystemStatus{}
		},
		status.NewSystemStatus,
		service.NewSystemService,
	),
)
