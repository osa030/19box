// Package main provides process management for 19box-server.
package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	zlog "github.com/rs/zerolog/log"
)

// ProcessManager manages 19box-server and child processes.
type ProcessManager struct {
	mu         sync.RWMutex
	serverCmd  *exec.Cmd
	children   []*exec.Cmd
	configPath string   // Temp config file path
	logDir     string   // Log output directory
	envVars    []string // Environment variables from .env
	running    bool
	doneCh     chan struct{}
}

// NewProcessManager creates a new ProcessManager.
func NewProcessManager(logDir string) *ProcessManager {
	return &ProcessManager{
		logDir: logDir,
		doneCh: make(chan struct{}),
	}
}

// IsRunning returns true if the server is running.
func (pm *ProcessManager) IsRunning() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.running
}

// StartServer starts the 19box-server and hook processes.
func (pm *ProcessManager) StartServer(
	ctx context.Context,
	serverPath string,
	configPath string,
	envVars []string,
	hooks []ProcessConfig,
) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.running {
		return errors.New("server is already running")
	}

	// Ensure log directory exists
	if err := os.MkdirAll(pm.logDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create log directory")
	}

	// Store config path for cleanup
	pm.configPath = configPath
	pm.envVars = envVars

	// Create environment for child processes
	env := os.Environ()
	for _, e := range envVars {
		env = append(env, e)
	}

	// Start 19box-server
	serverLogPath := filepath.Join(pm.logDir, "19box-server.log")

	// Ensure log file is fresh (truncate before starting)
	if f, err := os.OpenFile(serverLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err == nil {
		f.Close()
	}

	pm.serverCmd = exec.CommandContext(ctx, serverPath, "--config", configPath, "--logfile", serverLogPath)
	pm.serverCmd.Env = env
	// Stdout/Stderr are handled by 19box-server internally via --logfile
	// Set process group for proper cleanup
	pm.serverCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	zlog.Info().
		Str("path", serverPath).
		Str("config", configPath).
		Str("logfile", serverLogPath).
		Msg("Starting 19box-server")

	if err := pm.serverCmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start 19box-server")
	}

	// Start hook processes
	pm.children = make([]*exec.Cmd, 0, len(hooks))
	for _, hook := range hooks {
		logPath := filepath.Join(pm.logDir, sanitizeFilename(hook.Name)+".log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			zlog.Error().Err(err).Str("name", hook.Name).Msg("Failed to open log file for hook")
			continue
		}

		cmd := exec.CommandContext(ctx, hook.Command, hook.Args...)
		cmd.Env = env
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		zlog.Info().
			Str("name", hook.Name).
			Str("command", hook.Command).
			Msg("Starting hook process")

		if err := cmd.Start(); err != nil {
			logFile.Close()
			zlog.Error().Err(err).Str("name", hook.Name).Msg("Failed to start hook process")
			continue
		}

		pm.children = append(pm.children, cmd)
	}

	pm.running = true
	pm.doneCh = make(chan struct{})

	// Monitor server process
	go pm.monitorServer()

	return nil
}

// StopServer stops all processes gracefully.
func (pm *ProcessManager) StopServer() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.running {
		return errors.New("server is not running")
	}

	zlog.Info().Msg("Stopping all processes")

	// Stop children first
	for _, cmd := range pm.children {
		if cmd.Process != nil {
			// Send SIGTERM to process group
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
	}

	// Wait a bit for graceful shutdown
	time.Sleep(500 * time.Millisecond)

	// Stop server
	if pm.serverCmd != nil && pm.serverCmd.Process != nil {
		syscall.Kill(-pm.serverCmd.Process.Pid, syscall.SIGTERM)
	}

	// Wait for processes to terminate
	timeout := time.After(5 * time.Second)
	done := make(chan struct{})

	go func() {
		if pm.serverCmd != nil {
			pm.serverCmd.Wait()
		}
		for _, cmd := range pm.children {
			cmd.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
		zlog.Info().Msg("All processes stopped gracefully")
	case <-timeout:
		zlog.Warn().Msg("Timeout waiting for processes, sending SIGKILL")
		// Force kill
		if pm.serverCmd != nil && pm.serverCmd.Process != nil {
			syscall.Kill(-pm.serverCmd.Process.Pid, syscall.SIGKILL)
		}
		for _, cmd := range pm.children {
			if cmd.Process != nil {
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		}
	}

	// Cleanup temp config file
	if pm.configPath != "" {
		if err := os.Remove(pm.configPath); err != nil {
			zlog.Warn().Err(err).Str("path", pm.configPath).Msg("Failed to remove temp config")
		} else {
			zlog.Info().Str("path", pm.configPath).Msg("Removed temp config file")
		}
	}

	pm.running = false
	pm.serverCmd = nil
	pm.children = nil
	pm.configPath = ""
	close(pm.doneCh)

	return nil
}

// Done returns a channel that is closed when the server stops.
func (pm *ProcessManager) Done() <-chan struct{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.doneCh
}

// monitorServer monitors the server process and updates state on exit.
func (pm *ProcessManager) monitorServer() {
	if pm.serverCmd == nil {
		return
	}

	err := pm.serverCmd.Wait()
	
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.running {
		if err != nil {
			zlog.Error().Err(err).Msg("19box-server exited with error")
		} else {
			zlog.Info().Msg("19box-server exited normally")
		}
		pm.running = false
		
		// Stop children when server exits
		for _, cmd := range pm.children {
			if cmd.Process != nil {
				syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			}
		}
		
		// Cleanup temp config
		if pm.configPath != "" {
			os.Remove(pm.configPath)
		}
		
		close(pm.doneCh)
	}
}

// GetServerLog reads the last n lines from the server log.
func (pm *ProcessManager) GetServerLog(lines int) ([]string, error) {
	logPath := filepath.Join(pm.logDir, "19box-server.log")
	return tailFile(logPath, lines)
}

// tailFile reads the last n lines from a file.
func tailFile(path string, n int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Seek to end
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	if size == 0 {
		return []string{}, nil
	}

	// Read from end
	bufSize := int64(4096)
	if bufSize > size {
		bufSize = size
	}

	var lines []string
	offset := size

	for len(lines) < n && offset > 0 {
		readSize := bufSize
		if offset < bufSize {
			readSize = offset
		}
		offset -= readSize

		file.Seek(offset, io.SeekStart)
		buf := make([]byte, readSize)
		file.Read(buf)

		// Split into lines
		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] == '\n' && i < len(buf)-1 {
				lines = append([]string{string(buf[i+1:])}, lines...)
				buf = buf[:i]
				if len(lines) >= n {
					break
				}
			}
		}
		if offset == 0 && len(buf) > 0 {
			lines = append([]string{string(buf)}, lines...)
		}
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines, nil
}

// sanitizeFilename removes or replaces invalid filename characters.
func sanitizeFilename(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else if c == ' ' {
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "process"
	}
	return string(result)
}
