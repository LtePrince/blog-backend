package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"blog-backend/internal/blog/repository"
	"blog-backend/internal/blog/service"
	"blog-backend/internal/config"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
)

// HttpServer wraps Gin and exposes the blog HTTP API.
type HttpServer struct {
	engine      *gin.Engine
	srv         *http.Server
	blogService *service.BlogService
}

// NewHttpServer creates the HTTP server, registers routes, and hooks into fx lifecycle.
func NewHttpServer(
	lc fx.Lifecycle,
	cfg *config.Config,
	blogService *service.BlogService,
	blogRepo repository.IBlogRepository,
) *HttpServer {
	// Auto-migrate blog table on startup.
	if err := blogRepo.AutoMigrate(); err != nil {
		log.Fatalf("auto-migrate failed: %v", err)
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
		engine:      r,
		srv:         srv,
		blogService: blogService,
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
	}
}

// Module provides the HttpServer to the fx dependency graph.
var Module = fx.Module("server",
	fx.Provide(NewHttpServer),
	fx.Invoke(func(*HttpServer) {}), // ensure server is started
)
