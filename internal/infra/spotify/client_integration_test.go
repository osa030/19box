//go:build integration

package spotify

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIntegrationClient(t *testing.T) *Client {
	t.Helper()
	// Resolve .env path relative to this test file
	_, filename, _, _ := runtime.Caller(0)
	envPath := filepath.Join(filepath.Dir(filename), "../../../.env")
	_ = godotenv.Load(envPath)

	cfg := Config{
		ClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		RefreshToken: os.Getenv("SPOTIFY_REFRESH_TOKEN"),
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		t.Skip("Spotify credentials not set, skipping integration test")
	}
	client, err := New(context.Background(), cfg)
	require.NoError(t, err)
	return client
}

func TestIntegration_Search(t *testing.T) {
	client := setupIntegrationClient(t)
	ctx := context.Background()

	tracks, err := client.Search(ctx, "Bohemian Rhapsody", "track", 5)
	require.NoError(t, err)
	assert.NotEmpty(t, tracks)
	assert.NotEmpty(t, tracks[0].ID)
	assert.NotEmpty(t, tracks[0].Name)
	assert.NotEmpty(t, tracks[0].Artists)
	t.Logf("Search result[0]: %s - %v", tracks[0].Name, tracks[0].Artists)
}

func TestIntegration_PlaylistCRUD(t *testing.T) {
	client := setupIntegrationClient(t)
	ctx := context.Background()

	// Use Search to get a real track ID
	tracks, err := client.Search(ctx, "Bohemian Rhapsody Queen", "track", 1)
	require.NoError(t, err)
	require.NotEmpty(t, tracks)
	trackID := tracks[0].ID

	// CreatePlaylist
	playlistID, err := client.CreatePlaylist(ctx, "19box-integration-test", "Integration test playlist - safe to delete")
	require.NoError(t, err)
	assert.NotEmpty(t, playlistID)
	t.Logf("Created playlist: %s", client.GetPlaylistURL(playlistID))
	t.Log("*** Please manually delete this playlist after the test ***")

	// AddTracksToPlaylist
	err = client.AddTracksToPlaylist(ctx, playlistID, []string{trackID})
	require.NoError(t, err)
	t.Logf("Added track: %s (%s)", tracks[0].Name, trackID)

	// RemoveTracksFromPlaylist
	err = client.RemoveTracksFromPlaylist(ctx, playlistID, []string{trackID})
	require.NoError(t, err)
	t.Logf("Removed track: %s (%s)", tracks[0].Name, trackID)
}
