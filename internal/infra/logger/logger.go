// Package logger provides structured logging using zerolog.
package logger

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

// Config represents logger configuration.
type Config struct {
	Output string // "stdout", "stderr", or file path
	Level  string // "debug", "info", "warn", "error"
	File   string // log file path (used when Output is not stdout/stderr)
}

// Init initializes the global zerolog logger with the given configuration.
func Init(cfg Config) error {
	level := parseLevel(cfg.Level)

	var writer io.Writer
	switch strings.ToLower(cfg.Output) {
	case "stdout", "":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		// File output
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		writer = f
	}

	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.TimeOnly
	zerolog.TimestampFieldName = "time"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "message"

	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		parts := strings.Split(file, string(filepath.Separator))
		if len(parts) > 1 {
			return filepath.Join(parts[len(parts)-2:]...) + ":" + strconv.Itoa(line)
		}
		return filepath.Base(file) + ":" + strconv.Itoa(line)
	}

	// Set global logger
	// Use ConsoleWriter for stdout/stderr (color output), JSON for files
	var logger zerolog.Logger
	if strings.ToLower(cfg.Output) == "stdout" || strings.ToLower(cfg.Output) == "stderr" || cfg.Output == "" {
		// Console output with colors
		if level == zerolog.DebugLevel {
			// Add Caller only for DEBUG level
			logger = zerolog.New(zerolog.ConsoleWriter{
				Out:        writer,
				TimeFormat: time.TimeOnly,
				PartsOrder: []string{"time", "level", "message", "caller"},
				FormatCaller: func(i interface{}) string {
					return "(" + i.(string) + ")"
				},
			}).With().Timestamp().Caller().Logger()
		} else {
			logger = zerolog.New(zerolog.ConsoleWriter{
				Out:        writer,
				TimeFormat: time.TimeOnly,
			}).With().Timestamp().Logger()
		}
	} else {
		// JSON output for files
		baseLogger := zerolog.New(writer).With().Timestamp()
		if level == zerolog.DebugLevel {
			logger = baseLogger.Caller().Logger()
		} else {
			logger = baseLogger.Logger()
		}
	}
	zerolog.DefaultContextLogger = &logger
	zlog.Logger = logger

	return nil
}

// parseLevel parses the log level string.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info", "":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
