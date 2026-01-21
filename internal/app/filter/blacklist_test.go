package filter

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
	"github.com/osa030/19box/internal/infra/blacklist"
)

func createTestBlacklist(t *testing.T, content string) *blacklist.Blacklist {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "blacklist.yaml")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)

	bl, err := blacklist.Load(path)
	require.NoError(t, err)
	return bl
}

func TestBlacklistTrackFilter_Check(t *testing.T) {
	yamlContent := `
tracks:
  - "blacklisted_id"
  - "another_id" # with comment
`
	bl := createTestBlacklist(t, yamlContent)
	f := NewBlacklistTrackFilter(bl)

	tests := []struct {
		name     string
		trackID  string
		expected bool
	}{
		{
			name:     "Blacklisted track",
			trackID:  "blacklisted_id",
			expected: false,
		},
		{
			name:     "Another blacklisted track",
			trackID:  "another_id",
			expected: false,
		},
		{
			name:     "Allowed track",
			trackID:  "allowed_id",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run(tt.name, func(t *testing.T) {
				result := f.Check(context.Background(), TrackRequest{}, track.Track{ID: tt.trackID}, &listener.Session{})
				assert.Equal(t, tt.expected, result.Accepted)
				if !tt.expected {
					assert.Equal(t, "blacklisted_track", result.Code)
				}
			})
		})
	}
}

func TestBlacklistArtistFilter_Check(t *testing.T) {
	yamlContent := `
artists:
  - "banned_artist_id"
  - "another_artist_id"
`
	bl := createTestBlacklist(t, yamlContent)
	f := NewBlacklistArtistFilter(bl)

	tests := []struct {
		name      string
		artistIDs []string
		expected  bool
	}{
		{
			name:      "Blacklisted artist",
			artistIDs: []string{"banned_artist_id"},
			expected:  false,
		},
		{
			name:      "Allowed artist",
			artistIDs: []string{"allowed_artist_id"},
			expected:  true,
		},
		{
			name:      "Mixed artists (one blacklisted)",
			artistIDs: []string{"allowed_id", "banned_artist_id"},
			expected:  false,
		},
		{
			name:      "Multiple allowed artists",
			artistIDs: []string{"allowed_1", "allowed_2"},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.Check(context.Background(), TrackRequest{}, track.Track{ArtistIDs: tt.artistIDs}, &listener.Session{})
			assert.Equal(t, tt.expected, result.Accepted)
			if !tt.expected {
				assert.Equal(t, "blacklisted_artist", result.Code)
			}
		})
	}
}
