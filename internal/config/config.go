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
	App      AppConfig      `mapstructure:"app"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"` // sqlite, postgres, mysql
	DSN    string `mapstructure:"dsn"`
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

	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "blog.db")

	v.SetDefault("app.name", "blog-backend")
	v.SetDefault("app.environment", "dev")
}

// Module provides Config to the fx dependency graph.
var Module = fx.Module("config",
	fx.Provide(Load),
)
