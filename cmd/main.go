package main

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"blog-backend/internal/blog"
	"blog-backend/internal/config"
	"blog-backend/internal/moments"
	"blog-backend/internal/resource"
	"blog-backend/internal/server"
	"blog-backend/internal/system"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

var (
	configFile string
	GoVersion  = runtime.Version()
)

var rootCmd = &cobra.Command{
	Use:   "blog-backend",
	Short: "Whalefall Blog Backend",
	Long:  `Whalefall Blog Backend — personal blog API server`,
	Run: func(cmd *cobra.Command, args []string) {
		serveCmd.Run(cmd, args)
	},
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "start the blog backend server",
	Run: func(cmd *cobra.Command, args []string) {
		app := fx.New(
			// Provide the config file path to the DI container.
			fx.Provide(func() string { return configFile }),
			config.Module,
			resource.Module,
			blog.Module,
			system.Module,
			moments.Module,
			server.Module,
			fx.Invoke(func(cfg *config.Config) {
				log.Println("🐋 Whalefall Blog Server Started")
				log.Printf("   Environment : %s", cfg.App.Environment)
				log.Printf("   Go Version  : %s", GoVersion)
				log.Printf("   Listening   : %s:%d", cfg.Server.Host, cfg.Server.Port)
			}),
		)
		app.Run()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "show version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Whalefall Blog Backend\n")
		fmt.Printf("Go Version: %s\n", GoVersion)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "",
		"config file path (default: ./configs/config.yaml)")
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
