package playlist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/osa030/19box/internal/domain/track"
)

func TestPlaylist_TrackIDs(t *testing.T) {
	tests := []struct {
		name     string
		tracks   []track.Track
		expected []string
	}{
		{
			name:     "empty playlist",
			tracks:   []track.Track{},
			expected: []string{},
		},
		{
			name: "single track",
			tracks: []track.Track{
				{ID: "track-1"},
			},
			expected: []string{"track-1"},
		},
		{
			name: "multiple tracks",
			tracks: []track.Track{
				{ID: "track-1"},
				{ID: "track-2"},
				{ID: "track-3"},
			},
			expected: []string{"track-1", "track-2", "track-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Playlist{
				ID:     "playlist-1",
				Tracks: tt.tracks,
			}

			result := p.TrackIDs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPlaylist_TotalDuration(t *testing.T) {
	tests := []struct {
		name     string
		tracks   []track.Track
		expected int64
	}{
		{
			name:     "empty playlist",
			tracks:   []track.Track{},
			expected: 0,
		},
		{
			name: "single track",
			tracks: []track.Track{
				{ID: "track-1", Duration: 3 * time.Minute}, // 180 seconds
			},
			expected: 180,
		},
		{
			name: "multiple tracks",
			tracks: []track.Track{
				{ID: "track-1", Duration: 2 * time.Minute},                // 120 seconds
				{ID: "track-2", Duration: 3*time.Minute + 30*time.Second}, // 210 seconds
				{ID: "track-3", Duration: 4 * time.Minute},                // 240 seconds
			},
			expected: 570, // 120 + 210 + 240
		},
		{
			name: "tracks with fractional seconds",
			tracks: []track.Track{
				{ID: "track-1", Duration: 2*time.Minute + 15*time.Second}, // 135 seconds
				{ID: "track-2", Duration: 3*time.Minute + 45*time.Second}, // 225 seconds
			},
			expected: 360, // 135 + 225
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Playlist{
				ID:     "playlist-1",
				Name:   "Test Playlist",
				Tracks: tt.tracks,
			}

			result := p.TotalDuration()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPlaylist_Properties(t *testing.T) {
	p := &Playlist{
		ID:          "playlist-123",
		Name:        "My Awesome Playlist",
		Description: "A test playlist",
		URL:         "https://open.spotify.com/playlist/playlist-123",
		Tracks: []track.Track{
			{ID: "track-1", Name: "Song 1", Duration: 3 * time.Minute},
			{ID: "track-2", Name: "Song 2", Duration: 4 * time.Minute},
		},
	}

	assert.Equal(t, "playlist-123", p.ID)
	assert.Equal(t, "My Awesome Playlist", p.Name)
	assert.Equal(t, "A test playlist", p.Description)
	assert.Equal(t, "https://open.spotify.com/playlist/playlist-123", p.URL)
	assert.Len(t, p.Tracks, 2)
	assert.Equal(t, int64(420), p.TotalDuration()) // 180 + 240
}
