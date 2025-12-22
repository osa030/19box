package filter

import (
	"context"
	"testing"
	"time"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
	"github.com/stretchr/testify/assert"
)

func TestDurationLimitFilter_Check(t *testing.T) {
	tests := []struct {
		name          string
		minMinutes    float64
		maxMinutes    float64
		trackDuration time.Duration
		shouldReject  bool
		description   string
	}{
		{
			name:          "Within limits",
			minMinutes:    2.0,
			maxMinutes:    5.0,
			trackDuration: 3 * time.Minute,
			shouldReject:  false,
			description:   "Should accept track within min/max limits",
		},
		{
			name:          "Too short",
			minMinutes:    3.0,
			maxMinutes:    0,
			trackDuration: 2 * time.Minute,
			shouldReject:  true,
			description:   "Should reject track shorter than min",
		},
		{
			name:          "Too long",
			minMinutes:    1.0,
			maxMinutes:    5.0,
			trackDuration: 6 * time.Minute,
			shouldReject:  true,
			description:   "Should reject track longer than max",
		},
		{
			name:          "Exact min",
			minMinutes:    3.0,
			maxMinutes:    0,
			trackDuration: 3 * time.Minute,
			shouldReject:  false,
			description:   "Should accept track exactly at min",
		},
		{
			name:          "Exact max",
			minMinutes:    1.0,
			maxMinutes:    5.0,
			trackDuration: 5 * time.Minute,
			shouldReject:  false,
			description:   "Should accept track exactly at max",
		},
		{
			name:          "Below minimum",
			minMinutes:    2.0,
			maxMinutes:    0,
			trackDuration: 1 * time.Minute,
			shouldReject:  true,
			description:   "Should reject track below minimum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDurationLimitFilter()
			// Manually configuring for test by setting config directly
			f.config = &DurationLimitConfig{
				MinMinutes: tt.minMinutes,
				MaxMinutes: tt.maxMinutes,
			}

			result := f.Check(
				context.Background(),
				TrackRequest{},
				track.Track{Duration: tt.trackDuration},
				&listener.Session{},
			)

			if tt.shouldReject {
				assert.False(t, result.Accepted, tt.description)
				assert.Equal(t, "duration_limit_exceeded", result.Code)
			} else {
				assert.True(t, result.Accepted, tt.description)
			}
		})
	}
}

func TestDurationLimitFilter_ValidateConfig(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]interface{}
		wantErr  bool
		errCheck func(error) bool
	}{
		{
			name: "Valid config",
			settings: map[string]interface{}{
				"min_minutes": 2.5,
				"max_minutes": 5.0,
			},
			wantErr: false,
		},
		{
			name: "Valid integers",
			settings: map[string]interface{}{
				"min_minutes": 2,
				"max_minutes": 5,
			},
			wantErr: false,
		},
		{
			name: "Invalid min > max",
			settings: map[string]interface{}{
				"min_minutes": 10.0,
				"max_minutes": 5.0,
			},
			wantErr: true,
		},
		{
			name: "Invalid negative min",
			settings: map[string]interface{}{
				"min_minutes": -1.0,
			},
			wantErr: true,
		},
		{
			name: "Zero min (uses default min=1)",
			settings: map[string]interface{}{
				"min_minutes": 0.0,
			},
			wantErr: false,
		},
		{
			name: "Invalid min less than 1",
			settings: map[string]interface{}{
				"min_minutes": 0.5,
			},
			wantErr: true,
		},
		{
			name: "Zero max (allowed, means no limit)",
			settings: map[string]interface{}{
				"max_minutes": 0.0, // 0 is allowed and means no limit
			},
			wantErr: false,
		},
		{
			name: "Invalid negative max",
			settings: map[string]interface{}{
				"max_minutes": -1.0,
			},
			wantErr: true,
		},
		{
			name:     "Empty settings (uses defaults, min=1)",
			settings: map[string]interface{}{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDurationLimitFilter()
			err := f.ValidateConfig(tt.settings)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
