// Package main provides HTTP handlers for the webadmin API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	zlog "github.com/rs/zerolog/log"

	jukeboxv1 "github.com/osa030/19box/internal/gen/jukebox/v1"
	"github.com/osa030/19box/internal/gen/jukebox/v1/jukeboxv1connect"
	apiconnect "github.com/osa030/19box/internal/api/connect"
)

// Handler handles HTTP requests for the webadmin API.
type Handler struct {
	config     *WebAdminConfig
	baseConfig map[string]interface{}
	pm         *ProcessManager
	adminClient jukeboxv1connect.AdminServiceClient
}

// NewHandler creates a new Handler.
func NewHandler(cfg *WebAdminConfig, baseConfig map[string]interface{}, pm *ProcessManager) *Handler {
	return &Handler{
		config:     cfg,
		baseConfig: baseConfig,
		pm:         pm,
	}
}

// RegisterRoutes registers all API routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Config endpoints
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/config/preset/", h.handlePreset)

	// Server control endpoints
	mux.HandleFunc("/api/server/start", h.handleServerStart)
	mux.HandleFunc("/api/server/stop", h.handleServerStop)
	mux.HandleFunc("/api/server/status", h.handleServerStatus)

	// Session control endpoints (proxy to 19box-server)
	mux.HandleFunc("/api/session/pause", h.handleSessionPause)
	mux.HandleFunc("/api/session/resume", h.handleSessionResume)
	mux.HandleFunc("/api/session/skip", h.handleSessionSkip)
	mux.HandleFunc("/api/session/stop", h.handleSessionStop)

	// Listener endpoints
	mux.HandleFunc("/api/listeners", h.handleListeners)
	mux.HandleFunc("/api/listeners/", h.handleListenerKick)
}

// jsonResponse writes a JSON response.
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

// handleConfig returns base config and preset list.
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Build preset list
	presets := make([]map[string]string, 0, len(h.config.Presets))
	for name, preset := range h.config.Presets {
		presets = append(presets, map[string]string{
			"name":        name,
			"description": preset.Description,
		})
	}

	// Extract session, playlists, and filters from base config
	session := h.baseConfig["session"]
	playlists := h.baseConfig["playlists"]
	filters := h.baseConfig["filters"]

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"session":   session,
		"playlists": playlists,
		"server":    h.baseConfig["server"],
		"filters":   filters,
		"presets":   presets,
		"running":   h.pm.IsRunning(),
	})
}

// handlePreset returns config with preset applied.
func (h *Handler) handlePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract preset name from path
	name := strings.TrimPrefix(r.URL.Path, "/api/config/preset/")
	if name == "" {
		jsonError(w, http.StatusBadRequest, "preset name required")
		return
	}

	preset, ok := h.config.Presets[name]
	if !ok {
		jsonError(w, http.StatusNotFound, "preset not found")
		return
	}

	// Merge preset into base config
	merged := MergeConfig(h.baseConfig, preset)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"session":   merged["session"],
		"playlists": merged["playlists"],
		"server":    merged["server"],
		"filters":   merged["filters"],
	})
}

// StartRequest represents a server start request.
type StartRequest struct {
	Session   map[string]interface{} `json:"session"`
	Playlists map[string]interface{} `json:"playlists"`
	Server    map[string]interface{} `json:"server"`
	Filters   map[string]interface{} `json:"filters"`
}

// handleServerStart starts the 19box-server.
func (h *Handler) handleServerStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.pm.IsRunning() {
		jsonError(w, http.StatusConflict, "server is already running")
		return
	}

	// Parse request body
	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Merge form data into base config
	formData := map[string]interface{}{
		"session":   req.Session,
		"playlists": req.Playlists,
		"server":    req.Server,
		"filters":   req.Filters,
	}
	finalConfig := MergeFormData(h.baseConfig, formData)

	// Inject Admin Token from webadmin config
	if h.config.JukeBox.AdminToken != "" {
		if admin, ok := finalConfig["admin"].(map[string]interface{}); ok {
			admin["token"] = h.config.JukeBox.AdminToken
		} else {
			finalConfig["admin"] = map[string]interface{}{
				"token": h.config.JukeBox.AdminToken,
			}
		}
	}

	// Save to temp file
	configPath, err := SaveTempConfig(finalConfig)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to save temp config")
		jsonError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	// Load environment variables
	envVars, err := LoadEnvFile(h.config.JukeBox.EnvPath)
	if err != nil {
		zlog.Warn().Err(err).Msg("Failed to load .env file")
	}

	// Force ADMIN_TOKEN in env to match webadmin config
	// This ensures server uses the same token even if .env has a different one
	if h.config.JukeBox.AdminToken != "" {
		envVars = append(envVars, fmt.Sprintf("ADMIN_TOKEN=%s", h.config.JukeBox.AdminToken))
	}

	// Start server
	ctx := context.Background()
	if err := h.pm.StartServer(ctx, h.config.JukeBox.Path, configPath, envVars, h.config.Hooks.OnStart); err != nil {
		zlog.Error().Err(err).Msg("Failed to start server")
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Wait a moment for server to start
	time.Sleep(500 * time.Millisecond)

	if !h.pm.IsRunning() {
		logs, err := h.pm.GetServerLog(3) // Get last 3 lines
		msg := "Server exited immediately"
		if err == nil && len(logs) > 0 {
			msg = fmt.Sprintf("Server start failed: %s", strings.Join(logs, " | "))
		}
		jsonError(w, http.StatusInternalServerError, msg)
		return
	}

	// Create admin client for future requests
	h.adminClient = jukeboxv1connect.NewAdminServiceClient(
		http.DefaultClient,
		h.getServerURL(),
	)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Server started",
		"config":  configPath,
	})
}

// handleServerStop stops the 19box-server.
func (h *Handler) handleServerStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !h.pm.IsRunning() {
		jsonError(w, http.StatusConflict, "server is not running")
		return
	}

	// First, try to stop gracefully via gRPC
	if h.adminClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		
		_, err := h.adminClient.StopSession(ctx, withAuth(h, connect.NewRequest(&jukeboxv1.StopSessionRequest{})))
		if err != nil {
			zlog.Warn().Err(err).Msg("Failed to stop session via gRPC, forcing stop")
		}
	}

	// Then stop the process
	if err := h.pm.StopServer(); err != nil {
		zlog.Error().Err(err).Msg("Failed to stop server")
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.adminClient = nil

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Server stopped",
	})
}

// withAuth adds authentication header to the request
func withAuth[T any](h *Handler, req *connect.Request[T]) *connect.Request[T] {
	req.Header().Set(apiconnect.AdminTokenHeader, h.config.JukeBox.AdminToken)
	return req
}

// getServerURL returns the 19box-server API URL from base config.
func (h *Handler) getServerURL() string {
	addr := ":8080"
	if server, ok := h.baseConfig["server"].(map[string]interface{}); ok {
		if a, ok := server["addr"].(string); ok && a != "" {
			addr = a
		}
	}

	// Convert addr to full URL
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	} else if !strings.Contains(addr, ":") {
		addr = "localhost:" + addr
	}

	return "http://" + addr
}

// handleServerStatus returns the current server status.
func (h *Handler) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !h.pm.IsRunning() {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"running": false,
		})
		return
	}

	// Get status from 19box-server
	if h.adminClient == nil {
		h.adminClient = jukeboxv1connect.NewAdminServiceClient(
			http.DefaultClient,
			h.getServerURL(),
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := h.adminClient.GetStatus(ctx, withAuth(h, connect.NewRequest(&jukeboxv1.GetStatusRequest{})))
	if err != nil {
		// Log less verbosely for expected errors during startup/shutdown
		zlog.Warn().Err(err).Msg("Failed to get status from server")
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"running": true,
			"error":   "Failed to get status",
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"running":       true,
		"queueSize":     resp.Msg.QueueSize,
		"listenerCount": resp.Msg.ListenerCount,
		"sessionInfo":   resp.Msg.SessionInfo,
		"currentTrack":  resp.Msg.CurrentTrack,
	})
}

// handleSessionPause pauses the session.
func (h *Handler) handleSessionPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.adminClient == nil {
		jsonError(w, http.StatusServiceUnavailable, "server not running")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := h.adminClient.Pause(ctx, withAuth(h, connect.NewRequest(&jukeboxv1.PauseRequest{})))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": resp.Msg.Success,
		"message": resp.Msg.Message,
	})
}

// handleSessionResume resumes the session.
func (h *Handler) handleSessionResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.adminClient == nil {
		jsonError(w, http.StatusServiceUnavailable, "server not running")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := h.adminClient.Resume(ctx, withAuth(h, connect.NewRequest(&jukeboxv1.ResumeRequest{})))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": resp.Msg.Success,
		"message": resp.Msg.Message,
	})
}

// handleSessionSkip skips the current track.
func (h *Handler) handleSessionSkip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.adminClient == nil {
		jsonError(w, http.StatusServiceUnavailable, "server not running")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := h.adminClient.Skip(ctx, withAuth(h, connect.NewRequest(&jukeboxv1.SkipRequest{})))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": resp.Msg.Success,
		"message": resp.Msg.Message,
	})
}

// handleSessionStop stops the current session (graceful).
func (h *Handler) handleSessionStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.adminClient == nil {
		jsonError(w, http.StatusServiceUnavailable, "server not running")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := h.adminClient.StopSession(ctx, withAuth(h, connect.NewRequest(&jukeboxv1.StopSessionRequest{})))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": resp.Msg.Success,
		"message": resp.Msg.Message,
	})
}

// handleListeners returns the list of listeners.
func (h *Handler) handleListeners(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.adminClient == nil {
		jsonError(w, http.StatusServiceUnavailable, "server not running")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := h.adminClient.ListListeners(ctx, withAuth(h, connect.NewRequest(&jukeboxv1.ListListenersRequest{})))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"listeners": resp.Msg.Listeners,
	})
}

// handleListenerKick kicks a listener.
func (h *Handler) handleListenerKick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract listener ID from path: /api/listeners/{id}/kick
	path := strings.TrimPrefix(r.URL.Path, "/api/listeners/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "kick" {
		jsonError(w, http.StatusBadRequest, "invalid path")
		return
	}
	listenerID := parts[0]

	if h.adminClient == nil {
		jsonError(w, http.StatusServiceUnavailable, "server not running")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := h.adminClient.Kick(ctx, withAuth(h, connect.NewRequest(&jukeboxv1.KickRequest{
		ListenerId: listenerID,
	})))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": resp.Msg.Success,
		"message": resp.Msg.Message,
	})
}
