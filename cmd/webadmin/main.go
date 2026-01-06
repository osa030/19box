// Package main provides the webadmin server entry point.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/alecthomas/kingpin/v2"
	"github.com/joho/godotenv"
	zlog "github.com/rs/zerolog/log"

	"github.com/osa030/19box/internal/infra/logger"
	"github.com/osa030/19box/internal/infra/timezone"
)

//go:embed static/*
var staticFiles embed.FS

var (
	app        = kingpin.New("19box-webadmin", "19box Web Admin Server")
	configPath = app.Flag("config", "Path to webadmin config file").Default("config/webadmin.yaml").String()
	verbose    = app.Flag("verbose", "Enable verbose (DEBUG) logging").Short('v').Bool()
)

func main() {
	// Initialize timezone for Android (no-op on other platforms)
	timezone.Init()

	// Load .env file if it exists (errors are ignored)
	_ = godotenv.Load()

	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Initialize logger
	logLevel := "info"
	if *verbose {
		logLevel = "debug"
	}
	if err := logger.Init(logger.Config{
		Output: "stdout",
		Level:  logLevel,
	}); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Load webadmin config
	zlog.Info().Str("path", *configPath).Msg("Loading webadmin config")
	cfg, err := LoadWebAdminConfig(*configPath)
	if err != nil {
		zlog.Fatal().Err(err).Msg("Failed to load webadmin config")
	}

	// Load base server config
	zlog.Info().Str("path", cfg.JukeBox.BaseConfig).Msg("Loading base server config")
	baseConfig, err := LoadBaseConfig(cfg.JukeBox.BaseConfig)
	if err != nil {
		zlog.Fatal().Err(err).Msg("Failed to load base server config")
	}

	// Create log directory
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		zlog.Fatal().Err(err).Msg("Failed to create log directory")
	}

	// Create process manager
	pm := NewProcessManager(logDir)

	// Create handler
	handler := NewHandler(cfg, baseConfig, pm)

	// Create HTTP mux
	mux := http.NewServeMux()

	// Register API routes
	handler.RegisterRoutes(mux)

	// Serve static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		zlog.Fatal().Err(err).Msg("Failed to create static file system")
	}
	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("/", fileServer)

	// Create server
	server := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: mux,
	}

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		zlog.Info().Msg("Received shutdown signal")

		// Stop server if running
		if pm.IsRunning() {
			zlog.Info().Msg("Stopping 19box-server")
			if err := pm.StopServer(); err != nil {
				zlog.Error().Err(err).Msg("Failed to stop server")
			}
		}

		server.Close()
	}()

	// Get absolute path for display
	absPath, _ := filepath.Abs(*configPath)
	zlog.Info().
		Str("addr", cfg.Server.Addr).
		Str("config", absPath).
		Str("baseConfig", cfg.JukeBox.BaseConfig).
		Int("presets", len(cfg.Presets)).
		Msg("Starting 19box Web Admin")

	fmt.Printf("\n  🎵 19box Web Admin\n")
	fmt.Printf("  ──────────────────────────────────────\n")
	fmt.Printf("  URL:     http://localhost%s\n", cfg.Server.Addr)
	fmt.Printf("  Config:  %s\n", absPath)
	fmt.Printf("  Presets: %d\n", len(cfg.Presets))
	fmt.Printf("  ──────────────────────────────────────\n\n")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		zlog.Fatal().Err(err).Msg("Server error")
	}

	zlog.Info().Msg("Server stopped")
}
