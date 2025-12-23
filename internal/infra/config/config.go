// Package config provides configuration loading from YAML files.
package config

import (
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	Server    ServerConfig            `yaml:"server"`
	Session   SessionConfig           `yaml:"session"`
	Admin     AdminConfig             `yaml:"admin"`
	Playlists PlaylistsConfig         `yaml:"playlists"`
	Playback  PlaybackConfig          `yaml:"playback"`
	BGM       BGMConfig               `yaml:"bgm"`
	Filters   map[string]FilterConfig `yaml:"filters"`
	Messages  MessagesConfig          `yaml:"messages"`
	Spotify   SpotifyConfig           `yaml:"spotify"`
}

// ServerConfig represents server configuration.
type ServerConfig struct {
	Addr  string      `yaml:"addr" default:":8080"`
	Hooks HooksConfig `yaml:"hooks"`
}

// HooksConfig represents lifecycle hooks configuration.
type HooksConfig struct {
	OnStarted []string `yaml:"on_started"`
	OnStopped []string `yaml:"on_stopped"`
}



// SessionConfig represents session-related configuration.
type SessionConfig struct {
	Title     string   `yaml:"title"`
	StartTime string   `yaml:"start_time"`
	EndTime   string   `yaml:"end_time"`
	Keywords  []string `yaml:"keywords"`
}

// AdminConfig represents admin-related configuration.
type AdminConfig struct {
	Token        string   `yaml:"token" validate:"required"`
	DisplayNames []string `yaml:"display_names"`
}

// PlaylistsConfig represents playlist-related configuration.
type PlaylistsConfig struct {
	Opening PlaylistEntryConfig `yaml:"opening"`
	Ending  PlaylistEntryConfig `yaml:"ending"`
}

// PlaylistEntryConfig represents a single playlist configuration.
type PlaylistEntryConfig struct {
	PlaylistURL string `yaml:"playlist_url"`
	DisplayName string `yaml:"display_name" default:"DJ selection"`
}

// PlaybackConfig represents playback control configuration.
type PlaybackConfig struct {
	NotificationDelayMs int `yaml:"notification_delay_ms" default:"5000" validate:"gte=0,lte=30000"`
	GapCorrectionMs     int `yaml:"gap_correction_ms" default:"100" validate:"gte=0,lte=5000"`
}

// BGMConfig represents BGM configuration.
type BGMConfig struct {
	DepletionThresholdSec int              `yaml:"depletion_threshold_sec" default:"30"`
	RecentArtistCount     int              `yaml:"recent_artist_count" default:"3"`
	CandidateCount        int              `yaml:"candidate_count" default:"5"`
	Providers             []ProviderConfig `yaml:"providers" validate:"required,min=1"`
}

// ProviderConfig represents a single BGM provider configuration.
type ProviderConfig struct {
	Type        string         `yaml:"type" validate:"required"`
	DisplayName string         `yaml:"display_name" validate:"required"`
	Settings    map[string]any `yaml:"settings" validate:"required"`
}

// FilterConfig represents a filter's configuration.
type FilterConfig struct {
	Enabled  bool           `yaml:"enabled"`
	Settings map[string]any `yaml:"settings,omitempty"`
}

// MessagesConfig represents user-facing messages.
type MessagesConfig struct {
	Success               string `yaml:"success"`
	DefaultError          string `yaml:"default_error"`
	AcceptanceDone        string `yaml:"acceptance_done"`
	TimeLimitExceeded     string `yaml:"time_limit_exceeded"`
	Kicked                string `yaml:"kicked"`
	MarketRestriction     string `yaml:"market_restriction"`
	UserPending           string `yaml:"user_pending"`
	DuplicateTrack        string `yaml:"duplicate_track"`
	TrackNotFound         string `yaml:"track_not_found"`
	InvalidListener       string `yaml:"invalid_listener"`
	DurationLimitExceeded string `yaml:"duration_limit_exceeded"`
}

// SpotifyConfig represents Spotify API configuration.
type SpotifyConfig struct {
	ClientID     string `yaml:"client_id" validate:"required"`
	ClientSecret string `yaml:"client_secret" validate:"required"`
	RefreshToken string `yaml:"refresh_token" validate:"required"`
	Market       string `yaml:"market" validate:"omitempty,len=2" default:"JP"`
}

// Load loads configuration from a YAML file.
// Environment variables take precedence over file values for sensitive fields.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrap(err, "failed to parse config file")
	}

	// Override with environment variables
	cfg.overrideFromEnv()

	// Set defaults using creasty/defaults
	if err := defaults.Set(&cfg); err != nil {
		return nil, errors.Wrap(err, "failed to set defaults")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "config validation failed")
	}

	return &cfg, nil
}

// overrideFromEnv overrides config values with environment variables.
func (c *Config) overrideFromEnv() {
	if v := os.Getenv("SPOTIFY_CLIENT_ID"); v != "" {
		c.Spotify.ClientID = v
	}
	if v := os.Getenv("SPOTIFY_CLIENT_SECRET"); v != "" {
		c.Spotify.ClientSecret = v
	}
	if v := os.Getenv("SPOTIFY_REFRESH_TOKEN"); v != "" {
		c.Spotify.RefreshToken = v
	}
	if v := os.Getenv("LASTFM_API_KEY"); v != "" {
		for i := range c.BGM.Providers {
			if c.BGM.Providers[i].Type == "lastfm" {
				c.BGM.Providers[i].Settings["api_key"] = v
				break
			}
		}
	}
	if v := os.Getenv("ADMIN_TOKEN"); v != "" {
		c.Admin.Token = v
	}
}

// GetMessage returns the message for the given code.
func (c *Config) GetMessage(code string) string {
	switch code {
	case "success":
		return c.Messages.Success
	case "acceptance_done":
		return c.Messages.AcceptanceDone
	case "time_limit_exceeded":
		return c.Messages.TimeLimitExceeded
	case "kicked":
		return c.Messages.Kicked
	case "market_restriction":
		return c.Messages.MarketRestriction
	case "user_pending":
		return c.Messages.UserPending
	case "track_not_found":
		return c.Messages.TrackNotFound
	case "invalid_listener":
		return c.Messages.InvalidListener
	case "duplicate_track":
		return c.Messages.DuplicateTrack
	case "duration_limit_exceeded":
		return c.Messages.DurationLimitExceeded
	default:
		return c.Messages.DefaultError
	}
}

// IsAdminDisplayName checks if the given display name is an admin.
func (c *Config) IsAdminDisplayName(displayName string) bool {
	for _, name := range c.Admin.DisplayNames {
		if name == displayName {
			return true
		}
	}
	return false
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	validate := validator.New()
	if err := validate.Struct(c); err != nil {
		return errors.Wrap(err, "struct validation failed")
	}

	// Validate time consistency
	if err := c.validateTimeConsistency(); err != nil {
		return err
	}

	return nil
}

// validateTimeConsistency checks that end time is after start time and not in the past.
func (c *Config) validateTimeConsistency() error {
	now := time.Now()

	var startTime, endTime time.Time
	var hasStart, hasEnd bool

	// Check start_time if it's set
	if c.Session.StartTime != "" {
		st, err := time.Parse(time.RFC3339, c.Session.StartTime)
		if err != nil {
			return errors.Wrap(err, "failed to parse start_time")
		}
		startTime = st
		hasStart = true

		// Check if start_time is in the past
		if startTime.Before(now) {
			return errors.Newf("start_time (%s) must be in the future (current time: %s)", c.Session.StartTime, now.Format(time.RFC3339))
		}
	}

	// Check end_time if it's set
	if c.Session.EndTime != "" {
		et, err := time.Parse(time.RFC3339, c.Session.EndTime)
		if err != nil {
			return errors.Wrap(err, "failed to parse end_time")
		}
		endTime = et
		hasEnd = true

		// Check if end_time is in the past
		if endTime.Before(now) {
			return errors.Newf("end_time (%s) must be in the future (current time: %s)", c.Session.EndTime, now.Format(time.RFC3339))
		}
	}

	// Check if start_time is before end_time
	if hasStart && hasEnd {
		if !startTime.Before(endTime) {
			return errors.Newf("start_time (%s) must be before end_time (%s)", c.Session.StartTime, c.Session.EndTime)
		}
	}

	return nil
}

// ParseStartTime parses the start time string.
// Returns nil if the start time is empty.
func (c *Config) ParseStartTime() (*time.Time, error) {
	if c.Session.StartTime == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, c.Session.StartTime)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse start_time")
	}
	return &t, nil
}

// ParseEndTime parses the end time string.
// Returns nil if the end time is empty.
func (c *Config) ParseEndTime() (*time.Time, error) {
	if c.Session.EndTime == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, c.Session.EndTime)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse end_time")
	}
	return &t, nil
}

// IsFilterEnabled checks if a filter is enabled.
func (c *Config) IsFilterEnabled(filterName string) bool {
	if f, ok := c.Filters[filterName]; ok {
		return f.Enabled
	}
	return false
}

// GetFilterSettings returns the settings for a filter.
// not used currently
/*
func (c *Config) GetFilterSettings(filterName string) map[string]any {
	if f, ok := c.Filters[filterName]; ok {
		return f.Settings
	}
	return nil
}
*/
