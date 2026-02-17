package blog

import (
	"blog-backend/internal/blog/repository"
	"blog-backend/internal/blog/service"

	"go.uber.org/fx"
)

// Module provides blog repository and service to the fx dependency graph.
var Module = fx.Module("blog",
	fx.Provide(
		repository.NewBlogRepository,
		service.NewBlogService,
	),
)
