// Package main provides the server entry point.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/joho/godotenv"
	zlog "github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"connectrpc.com/connect"

	apiconnect "github.com/osa030/19box/internal/api/connect"
	"github.com/osa030/19box/internal/app/filter"
	"github.com/osa030/19box/internal/app/session"
	"github.com/osa030/19box/internal/gen/jukebox/v1/jukeboxv1connect"
	"github.com/osa030/19box/internal/infra/config"
	"github.com/osa030/19box/internal/infra/logger"
	"github.com/osa030/19box/internal/infra/spotify"
)

var (
	app        = kingpin.New("19box-server", "19box jukebox server")
	configPath = app.Flag("config", "Path to config file").Default("config/server.yaml").String()
	verbose    = app.Flag("verbose", "Enable verbose (DEBUG) logging").Short('v').Bool()
	logfile    = app.Flag("logfile", "Path to log file (default: stdout)").String()

	// list-filters command
	listFiltersCmd = app.Command("list-filters", "List available filters and exit")
)

func init() {
	// start command (default) - no need to store the command
	app.Command("start", "Start the server (default)").Default()
}

func main() {
	// Load .env file if it exists (errors are ignored)
	_ = godotenv.Load()

	// Parse command
	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	// Handle list-filters command
	if command == listFiltersCmd.FullCommand() {
		printFilters()
		return
	}

	// Initialize logger
	loggerConfig := logger.Config{
		Output: "stdout",
		Level:  "info",
		File:   "",
	}
	// Override with command-line flags if specified
	if *verbose {
		loggerConfig.Level = "debug"
	}
	if *logfile != "" {
		loggerConfig.Output = *logfile
		loggerConfig.File = *logfile
	}
	if err := logger.Init(loggerConfig); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Load config
	zlog.Info().Msgf("Loading config from %s", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		zlog.Fatal().Msgf("Failed to load config: %v", err)
	}

	// Run server (defer ensures shutdown hook is called)
	if err := run(cfg); err != nil {
		zlog.Error().Msgf("Server error: %v", err)
		os.Exit(1)
	}
}

// run executes the main server logic. Using a separate function ensures
// defer statements are executed even when returning with an error.
func run(cfg *config.Config) error {

	// Setup shutdown hook (defer ensures it runs on any exit from this function)


	// Validate filter config
	if err := validateFilterConfig(cfg); err != nil {
		return fmt.Errorf("invalid filter config: %w", err)
	}

	// Create Spotify client
	ctx := context.Background()
	spotifyClient, err := spotify.New(ctx, spotify.Config{
		ClientID:     cfg.Spotify.ClientID,
		ClientSecret: cfg.Spotify.ClientSecret,
		RefreshToken: cfg.Spotify.RefreshToken,
		Market:       cfg.Spotify.Market,
	})
	if err != nil {
		return fmt.Errorf("failed to create Spotify client: %w", err)
	}

	// Validate playlist existence
	if err := validatePlaylists(ctx, cfg, spotifyClient); err != nil {
		return fmt.Errorf("playlist validation failed: %w", err)
	}

	// Create session manager
	sessionMgr, err := session.NewManager(cfg, spotifyClient)
	if err != nil {
		return fmt.Errorf("failed to create session manager: %w", err)
	}

	// Create RPC services
	listenerService := apiconnect.NewListenerService(sessionMgr, cfg)
	adminService := apiconnect.NewAdminService(sessionMgr, cfg)

	// Create HTTP mux
	mux := http.NewServeMux()

	// Register services
	listenerPath, listenerHandler := jukeboxv1connect.NewListenerServiceHandler(listenerService)
	
	// Create admin auth interceptor
	adminAuthInterceptor := apiconnect.NewAdminAuthInterceptor(cfg)
	adminPath, adminHandler := jukeboxv1connect.NewAdminServiceHandler(
		adminService,
		connect.WithInterceptors(adminAuthInterceptor),
	)

	mux.Handle(listenerPath, listenerHandler)
	mux.Handle(adminPath, adminHandler)

	// Determine server address
	serverAddr := cfg.Server.Addr
	// Create server with h2c (HTTP/2 cleartext) support
	server := &http.Server{
		Addr:    serverAddr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	// Channel to capture server startup errors
	serverErrCh := make(chan error, 1)
	serverStartedCh := make(chan struct{})

	// Start session
	go func() {
		if err := sessionMgr.Start(ctx); err != nil {
			zlog.Error().Msgf("Failed to start session: %v", err)
		}
	}()

	// Start server
	go func() {
		zlog.Info().Msgf("Starting server: addr=%s", serverAddr)
		// Signal that we're about to start listening
		close(serverStartedCh)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()

	// Wait for server to start listening
	<-serverStartedCh
	// Give the server a moment to fully initialize
	time.Sleep(100 * time.Millisecond)

	// Execute startup hook if configured (after server is running)
	executeHooks(cfg.Server.Hooks.OnStarted, "on_started")


	// Wait for shutdown signal, session end, or server error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		zlog.Info().Msg("Received shutdown signal...")
		// Stop session immediately to send notifications (without ending playlist)
		if err := sessionMgr.StopImmediate(ctx); err != nil {
			zlog.Error().Msgf("Failed to stop session: %v", err)
		}
	case <-sessionMgr.Done():
		zlog.Info().Msg("Session ended, shutting down...")
	case err := <-serverErrCh:
		return fmt.Errorf("server error: %w", err)
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Close session manager first to terminate active connections/streams
	sessionMgr.Close()

	if err := server.Shutdown(shutdownCtx); err != nil {
		zlog.Error().Msgf("Failed to shutdown server: %v", err)
	}

	zlog.Info().Msg("Server stopped")

	// Execute shutdown hook if configured
	executeHooks(cfg.Server.Hooks.OnStopped, "on_stopped")

	return nil
}

// printFilters prints available filters.
func printFilters() {
	fmt.Println("Available Filters:")
	for _, factory := range filter.GetRegistered() {
		f := factory()
		codes := strings.Join(f.ReturnCodes(), ", ")
		fmt.Printf("  %-30s - %s [codes: %s]\n", f.Name(), f.Description(), codes)
	}
}

// validateFilterConfig validates filter configurations.
func validateFilterConfig(cfg *config.Config) error {
	registry := filter.GetRegistered()

	for filterName, filterCfg := range cfg.Filters {
		if !filterCfg.Enabled {
			continue
		}

		factory, exists := registry[filterName]
		if !exists {
			// Some filters are created with dependencies, skip validation
			continue
		}

		f := factory()
		if err := f.ValidateConfig(filterCfg.Settings); err != nil {
			return fmt.Errorf("filter %s: %w", filterName, err)
		}
	}

	return nil
}



// validatePlaylists validates that configured playlists exist on Spotify.
// This uses lightweight checks to avoid fetching all tracks during startup.
// It includes retry logic to handle transient errors during startup.
func validatePlaylists(ctx context.Context, cfg *config.Config, spotifyClient *spotify.Client) error {
	maxRetries := 5
	baseDelay := 1 * time.Second

	var errs []string

	// Helper function to validate a single playlist with retry
	validate := func(name, url string) error {
		zlog.Info().Msgf("Validating %s playlist: url=%s", name, url)

		var lastErr error
		for i := 0; i < maxRetries; i++ {
			if i > 0 {
				delay := baseDelay * time.Duration(1<<uint(i-1))
				zlog.Info().Msgf("Retrying %s playlist validation in %v...", name, delay)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}

			if err := spotifyClient.CheckPlaylistExists(ctx, url); err != nil {
				lastErr = err
				zlog.Warn().Msgf("Failed to validate %s playlist (attempt %d/%d): %v", name, i+1, maxRetries, err)
				continue
			}

			zlog.Info().Msgf("%s playlist validated successfully", strings.Title(name))
			return nil
		}
		return fmt.Errorf("failed after %d attempts: %v", maxRetries, lastErr)
	}

	// Validate opening playlist (optional)
	// Validate opening playlist (optional)
	if cfg.Playlists.Opening.PlaylistURL != "" {
		if err := validate("opening", cfg.Playlists.Opening.PlaylistURL); err != nil {
			errs = append(errs, fmt.Sprintf("opening playlist (%s): %v", cfg.Playlists.Opening.PlaylistURL, err))
		}
	} else {
		zlog.Info().Msg("Opening playlist not configured, session will start with BGM or user requests")
	}

	// Validate ending playlist (optional)
	if cfg.Playlists.Ending.PlaylistURL != "" {
		if err := validate("ending", cfg.Playlists.Ending.PlaylistURL); err != nil {
			errs = append(errs, fmt.Sprintf("ending playlist (%s): %v", cfg.Playlists.Ending.PlaylistURL, err))
		}
	} else {
		zlog.Info().Msg("Ending playlist not configured, session will end immediately after request acceptance stops")
	}

	if len(errs) > 0 {
		return fmt.Errorf("playlist validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// executeHooks runs a list of shell commands.
func executeHooks(hooks []string, stage string) {
	if len(hooks) == 0 {
		return
	}

	zlog.Info().Msgf("Executing %s hooks (%d commands)", stage, len(hooks))

	for _, hook := range hooks {
		zlog.Info().Msgf("Executing hook: %s", hook)
		// Use sh -c to allow shell features like redirection or pipes
		cmd := exec.Command("sh", "-c", hook)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			zlog.Error().Err(err).Msgf("Failed to execute hook: %s", hook)
		}
	}
}
