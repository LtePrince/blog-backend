package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"blog-backend/internal/blog/repository"
	blogservice "blog-backend/internal/blog/service"
	columnrepo "blog-backend/internal/column/repository"
	columnservice "blog-backend/internal/column/service"
	"blog-backend/internal/config"
	momentsservice "blog-backend/internal/moments/service"
	systemservice "blog-backend/internal/system/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
)

// HttpServer wraps Gin and exposes the blog HTTP API.
type HttpServer struct {
	engine         *gin.Engine
	srv            *http.Server
	blogService    *blogservice.BlogService
	systemService  *systemservice.SystemService
	momentsService *momentsservice.MomentsService
	columnService  *columnservice.ColumnService
}

// NewHttpServer creates the HTTP server, registers routes, and hooks into fx lifecycle.
func NewHttpServer(
	lc fx.Lifecycle,
	cfg *config.Config,
	blogService *blogservice.BlogService,
	systemService *systemservice.SystemService,
	momentsService *momentsservice.MomentsService,
	columnService *columnservice.ColumnService,
	blogRepo repository.IBlogRepository,
	columnRepo columnrepo.IColumnRepository,
) *HttpServer {
	// Auto-migrate blog & column tables on startup.
	if err := blogRepo.AutoMigrate(); err != nil {
		log.Fatalf("auto-migrate failed: %v", err)
	}
	if err := columnRepo.AutoMigrate(); err != nil {
		log.Fatalf("column auto-migrate failed: %v", err)
	}

	if cfg.App.Environment == "prd" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(corsMiddleware())

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: r,
	}

	h := &HttpServer{
		engine:         r,
		srv:            srv,
		blogService:    blogService,
		systemService:  systemService,
		momentsService: momentsService,
		columnService:  columnService,
	}
	h.registerRoutes(cfg)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Printf("🌐 HTTP server listening on %s", srv.Addr)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("HTTP server error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Println("Stopping HTTP server...")
			return srv.Shutdown(ctx)
		},
	})

	return h
}

// corsMiddleware adds permissive CORS headers for development.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// registerRoutes sets up all HTTP routes.
func (h *HttpServer) registerRoutes(cfg *config.Config) {
	h.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, Response{Code: "OK", Message: "healthy"})
	})

	// Serve static assets from the blog content repo.
	h.engine.Static("/static", cfg.Content.RepoDir)

	api := h.engine.Group("/api/v1")
	{
		// Site statistics
		api.GET("/stats", handleQuery(h.blogService.Stats))

		// Tags
		api.GET("/tags", handleQuery(h.blogService.ListTags))

		// Moments (one-line personal timeline)
		api.GET("/moments", handleQuery(h.momentsService.ListMoments))

		// System Info
		system := api.Group("/system")
		{
			system.GET("/stats", handleQuery(h.systemService.GetSystemStatus))
		}

		blogs := api.Group("/blogs")
		{
			// Read endpoints (frontend)
			blogs.GET("", handleQuery(h.blogService.ListBlogs))
			blogs.GET("/recent", handleQuery(h.blogService.RecentBlogs))
			blogs.GET("/:id", handleURI(h.blogService.GetBlog))

			// Write endpoints (CLI tool)
			blogs.POST("", handleJSON(h.blogService.CreateBlog))
			blogs.PUT("/:id", handleAll(h.blogService.UpdateBlog))
			blogs.DELETE("/:id", handleURI(h.blogService.DeleteBlog))
		}

		columns := api.Group("/columns")
		{
			// Read endpoints (frontend)
			columns.GET("", handleQuery(h.columnService.ListColumns))
			columns.GET("/tags", handleQuery(h.columnService.ListTags))
			columns.GET("/:slug", handleURI(h.columnService.GetColumn))
			columns.GET("/:slug/chapters/:chapter", handleURI(h.columnService.GetChapter))

			// Write endpoints (CLI tool)
			columns.POST("", handleJSON(h.columnService.CreateColumn))
			columns.PUT("/:slug", handleAll(h.columnService.UpdateColumn))
			columns.DELETE("/:slug", handleURI(h.columnService.DeleteColumn))
		}
	}
}

// Module provides the HttpServer to the fx dependency graph.
var Module = fx.Module("server",
	fx.Provide(NewHttpServer),
	fx.Invoke(func(*HttpServer) {}), // ensure server is started
)
