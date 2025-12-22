package filter

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
	zlog "github.com/rs/zerolog/log"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// DurationLimitConfig represents the configuration for DurationLimitFilter.
type DurationLimitConfig struct {
	MinMinutes float64 `yaml:"min_minutes" mapstructure:"min_minutes" default:"1" validate:"gte=1"`
	MaxMinutes float64 `yaml:"max_minutes" mapstructure:"max_minutes" validate:"gte=0"`
}

// DurationLimitFilter checks if track duration is within allowed limits.
type DurationLimitFilter struct {
	config *DurationLimitConfig
}

// NewDurationLimitFilter creates a new duration limit filter.
func NewDurationLimitFilter() *DurationLimitFilter {
	return &DurationLimitFilter{}
}

func (f *DurationLimitFilter) Name() string {
	return "duration_limit_filter"
}

func (f *DurationLimitFilter) Description() string {
	return "Checks if track duration is within allowed limits"
}

func (f *DurationLimitFilter) ReturnCodes() []string {
	return []string{"duration_limit_exceeded"}
}

func (f *DurationLimitFilter) ValidateConfig(settings map[string]any) error {
	var config DurationLimitConfig

	// Decode map[string]any to struct using mapstructure
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &config,
		TagName: "mapstructure",
	})
	if err != nil {
		return errors.Wrap(err, "failed to create decoder")
	}

	if err := decoder.Decode(settings); err != nil {
		return errors.Wrap(err, "failed to decode settings")
	}

	// Set defaults
	if err := defaults.Set(&config); err != nil {
		return errors.Wrap(err, "failed to set defaults")
	}

	// Validate using validator
	validate := validator.New()
	if err := validate.Struct(config); err != nil {
		return errors.Wrap(err, "validation failed")
	}

	// Custom validation: min_minutes cannot be greater than max_minutes
	if config.MaxMinutes > 0 && config.MinMinutes > config.MaxMinutes {
		return errors.New("min_minutes cannot be greater than max_minutes")
	}
	// Custom validation: max_minutes must be positive if set (0 means no limit)
	if config.MaxMinutes < 0 {
		return errors.New("max_minutes must be non-negative")
	}
	f.config = &config
	zlog.Info().Msgf("duration limit filter config: %+v", config)
	return nil
}

func (f *DurationLimitFilter) AppliesTo(requesterType track.RequesterType) bool {
	// Apply to user requests only
	return requesterType == track.RequesterTypeUser
}

func (f *DurationLimitFilter) Check(ctx context.Context, req TrackRequest, t track.Track, l *listener.Session) Result {
	// If config is not set, accept all tracks
	if f.config == nil {
		return Accept()
	}

	durationMinutes := t.Duration.Minutes()

	// Check minimum duration
	if durationMinutes < f.config.MinMinutes {
		return Reject("duration_limit_exceeded")
	}

	// Check maximum duration
	if f.config.MaxMinutes > 0 && durationMinutes > f.config.MaxMinutes {
		return Reject("duration_limit_exceeded")
	}

	return Accept()
}

func init() {
	Register("duration_limit_filter", func() Filter {
		return &DurationLimitFilter{}
	})
}
