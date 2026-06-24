package moments

import (
	"blog-backend/internal/moments/service"

	"go.uber.org/fx"
)

// Module provides the moments service to the fx dependency graph.
var Module = fx.Module("moments",
	fx.Provide(service.NewMomentsService),
)
