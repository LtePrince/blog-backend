package column

import (
	"blog-backend/internal/column/repository"
	"blog-backend/internal/column/service"
	"blog-backend/internal/config"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

// Module provides the column repository and service to the fx dependency graph.
var Module = fx.Module("column",
	fx.Provide(
		func(db *gorm.DB, cfg *config.Config) repository.IColumnRepository {
			return repository.NewColumnRepository(db, cfg.Content.RepoDir)
		},
		service.NewColumnService,
	),
)
