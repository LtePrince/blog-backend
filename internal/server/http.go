package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"

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

	if cfg.Auth.AdminToken == "" {
		log.Println("⚠️  no admin token configured (set BLOG_AUTH_ADMIN_TOKEN) — write endpoints are disabled")
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(corsMiddleware(cfg.Server.AllowedOrigins))

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

// corsMiddleware grants CORS access to the configured origins only.
//
// Both the blog frontend and blog-manager reach this API from their own
// server side, so the allowlist is normally empty and no origin is granted
// browser access. Add an origin here only when something genuinely needs it.
func corsMiddleware(allowed []string) gin.HandlerFunc {
	allowSet := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		allowSet[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowSet[origin]; ok && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
			// The response body varies per origin; keep caches from serving
			// one origin's response to another.
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// serveContent serves the blog content repo over /static, refusing any path
// with a dot-prefixed segment. The repo root holds .git and .github, and a
// plain file server would publish the entire repository history alongside the
// post images it is meant to serve.
func serveContent(root string) gin.HandlerFunc {
	fs := http.StripPrefix("/static", http.FileServer(http.Dir(root)))

	return func(c *gin.Context) {
		if hasDotSegment(c.Param("filepath")) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		fs.ServeHTTP(c.Writer, c.Request)
	}
}

// hasDotSegment reports whether any segment of p begins with a dot, which
// covers both repository metadata (.git) and traversal attempts ("..").
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(path.Clean(p), "/") {
		if seg != "." && strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// registerRoutes sets up all HTTP routes.
func (h *HttpServer) registerRoutes(cfg *config.Config) {
	h.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, Response{Code: "OK", Message: "healthy"})
	})

	// Serve static assets from the blog content repo.
	h.engine.GET("/static/*filepath", serveContent(cfg.Content.RepoDir))
	h.engine.HEAD("/static/*filepath", serveContent(cfg.Content.RepoDir))

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

		// Every write endpoint sits behind the admin token; reads stay public.
		requireAdmin := requireAdminToken(cfg.Auth.AdminToken)

		blogs := api.Group("/blogs")
		{
			// Read endpoints (frontend)
			blogs.GET("", handleQuery(h.blogService.ListBlogs))
			blogs.GET("/recent", handleQuery(h.blogService.RecentBlogs))
			blogs.GET("/:id", handleURI(h.blogService.GetBlog))

			// Write endpoints (blog-manager) — admin token required
			admin := blogs.Group("", requireAdmin)
			admin.POST("", handleJSON(h.blogService.CreateBlog))
			admin.PUT("/:id", handleAll(h.blogService.UpdateBlog))
			admin.DELETE("/:id", handleURI(h.blogService.DeleteBlog))
		}

		columns := api.Group("/columns")
		{
			// Read endpoints (frontend)
			columns.GET("", handleQuery(h.columnService.ListColumns))
			columns.GET("/tags", handleQuery(h.columnService.ListTags))
			columns.GET("/:slug", handleURI(h.columnService.GetColumn))
			columns.GET("/:slug/chapters/:chapter", handleURI(h.columnService.GetChapter))

			// Write endpoints (blog-manager) — admin token required
			admin := columns.Group("", requireAdmin)
			admin.POST("", handleJSON(h.columnService.CreateColumn))
			admin.PUT("/:slug", handleAll(h.columnService.UpdateColumn))
			admin.DELETE("/:slug", handleURI(h.columnService.DeleteColumn))
		}
	}
}

// Module provides the HttpServer to the fx dependency graph.
var Module = fx.Module("server",
	fx.Provide(NewHttpServer),
	fx.Invoke(func(*HttpServer) {}), // ensure server is started
)
