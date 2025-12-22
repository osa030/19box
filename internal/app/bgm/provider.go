// Package bgm provides BGM (background music) track provision strategies.
package bgm

import (
	"context"

	"github.com/osa030/19box/internal/domain/track"
)

// Provider is the interface for BGM track providers.
// Different implementations can provide tracks through various strategies
// (e.g., playlist-based, recommendation-based, etc.).
type Provider interface {
	// GetCandidates retrieves BGM track candidates.
	// count: the number of candidates to retrieve
	// seedTracks: recently played tracks that can be used as hints for recommendations
	// existingTrackIDs: tracks already in the queue (for duplicate avoidance)
	GetCandidates(ctx context.Context, count int, seedTracks []track.Track, existingTrackIDs map[string]bool) ([]track.Track, error)

	// Name returns the provider name (used in config).
	Name() string
}

// SpotifyClient defines the interface for Spotify operations needed by BGM providers.
type SpotifyClient interface {
	GetPlaylistTracksRandom(ctx context.Context, playlistURL string, count int) ([]track.Track, error)
	Search(ctx context.Context, query string, searchType string, limit int) ([]track.Track, error)
	GetTrack(ctx context.Context, trackID string, market ...string) (*track.Track, error)
}
