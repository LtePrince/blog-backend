package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/fx"
)

// Config holds all configuration for the application.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Content  ContentConfig  `mapstructure:"content"`
	Auth     AuthConfig     `mapstructure:"auth"`
	App      AppConfig      `mapstructure:"app"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	// AllowedOrigins lists the browser origins permitted to call the API
	// cross-origin. Both frontends talk to the backend server-side, so the
	// normal value is empty: no origin gets CORS access.
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// AuthConfig holds credentials for the write API.
type AuthConfig struct {
	// AdminToken is the shared secret blog-manager presents as
	// "Authorization: Bearer <token>" on every write request. An empty value
	// disables the write endpoints instead of leaving them open.
	//
	// Never commit a production value — set BLOG_AUTH_ADMIN_TOKEN instead.
	AdminToken string `mapstructure:"admin_token"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"` // sqlite, postgres, mysql
	DSN    string `mapstructure:"dsn"`
}

// ContentConfig holds paths to the blog content repository.
type ContentConfig struct {
	RepoDir string `mapstructure:"repo_dir"` // absolute path to blog repo root
}

// AppConfig holds application-level settings.
type AppConfig struct {
	Name        string `mapstructure:"name"`
	Environment string `mapstructure:"environment"` // dev, stg, prd
}

// Load reads configuration from file and environment variables.
func Load(configFile string) (*Config, error) {
	v := viper.New()

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
	}

	v.AutomaticEnv()
	v.SetEnvPrefix("BLOG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Bind the admin token explicitly. AutomaticEnv alone does not reliably
	// surface a key during Unmarshal when the config file omits it, which is
	// exactly the production case: the file is empty and the env supplies it.
	if err := v.BindEnv("auth.admin_token", "BLOG_AUTH_ADMIN_TOKEN"); err != nil {
		return nil, fmt.Errorf("failed to bind auth.admin_token: %w", err)
	}

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		if configFile != "" {
			return nil, fmt.Errorf("specified config file not found: %s", configFile)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.allowed_origins", []string{})

	v.SetDefault("auth.admin_token", "")

	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "blog.db")

	v.SetDefault("content.repo_dir", "./blogs")

	v.SetDefault("app.name", "blog-backend")
	v.SetDefault("app.environment", "dev")
}

// Module provides Config to the fx dependency graph.
var Module = fx.Module("config",
	fx.Provide(Load),
)
