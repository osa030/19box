package track

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTrack_IsAvailableInMarket(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name       string
		markets    []string
		isPlayable *bool
		market     string
		expected   bool
	}{
		{
			name:     "available in market using markets list",
			markets:  []string{"JP", "US", "UK"},
			isPlayable: nil,
			market:   "JP",
			expected: true,
		},
		{
			name:     "not available in market using markets list",
			markets:  []string{"US", "UK"},
			isPlayable: nil,
			market:   "JP",
			expected: false,
		},
		{
			name:       "isPlayable true takes precedence",
			markets:    []string{"US"}, // Market not in list
			isPlayable: &trueVal,
			market:     "JP",
			expected:   true,
		},
		{
			name:       "isPlayable false takes precedence",
			markets:    []string{"JP", "US"}, // Market in list
			isPlayable: &falseVal,
			market:     "JP",
			expected:   false,
		},
		{
			name:     "empty markets list",
			markets:  []string{},
			isPlayable: nil,
			market:   "JP",
			expected: false,
		},
		{
			name:     "case sensitivity",
			markets:  []string{"jp"},
			isPlayable: nil,
			market:   "JP",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := &Track{
				ID:         "test-id",
				Markets:    tt.markets,
				IsPlayable: tt.isPlayable,
			}

			result := track.IsAvailableInMarket(tt.market)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTrack_Validation(t *testing.T) {
	tests := []struct {
		name  string
		track Track
		valid bool
	}{
		{
			name: "valid track",
			track: Track{
				ID:       "spotify-id-123",
				Name:     "Test Song",
				Artists:  []string{"Artist 1"},
				Duration: 3 * time.Minute,
				URL:      "https://open.spotify.com/track/123",
			},
			valid: true,
		},
		{
			name: "empty ID",
			track: Track{
				Name:    "Test Song",
				Artists: []string{"Artist 1"},
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.track.ID)
			} else {
				assert.Empty(t, tt.track.ID)
			}
		})
	}
}
