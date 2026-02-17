package blog

import (
	"blog-backend/internal/blog/repository"
	"blog-backend/internal/blog/service"
	"blog-backend/internal/config"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

// Module provides blog repository and service to the fx dependency graph.
var Module = fx.Module("blog",
	fx.Provide(
		func(db *gorm.DB, cfg *config.Config) repository.IBlogRepository {
			return repository.NewBlogRepository(db, cfg.Content.RepoDir)
		},
		service.NewBlogService,
	),
)
