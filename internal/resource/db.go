package resource

import (
	"fmt"
	"log"

	"blog-backend/internal/config"

	"go.uber.org/fx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB creates a *gorm.DB based on the application config.
func NewDB(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.Database.Driver {
	case "sqlite":
		dialector = sqlite.Open(cfg.Database.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}

	logLevel := logger.Info
	if cfg.App.Environment == "prd" {
		logLevel = logger.Warn
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	log.Printf("✅ Database connected (driver=%s)", cfg.Database.Driver)
	return db, nil
}

// Module provides *gorm.DB to the fx dependency graph.
var Module = fx.Module("resource",
	fx.Provide(NewDB),
)
