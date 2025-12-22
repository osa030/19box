package bgm

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
	zlog "github.com/rs/zerolog/log"

	"github.com/osa030/19box/internal/domain/track"
)

type PlaylistProviderConfig struct {
	PlaylistURL string `yaml:"playlist_url" mapstructure:"playlist_url" validate:"required"`
}

// PlaylistProvider provides BGM tracks by randomly selecting from a configured playlist.
// It maintains an internal cache to minimize Spotify API calls.
type PlaylistProvider struct {
	spotify        SpotifyClient
	cache          []track.Track
	candidateCount int // Target cache size
	config         *PlaylistProviderConfig
}

// NewPlaylistProvider creates a new PlaylistProvider.
func NewPlaylistProvider(spotify SpotifyClient, candidateCount int, settings map[string]any) (*PlaylistProvider, error) {

	var config PlaylistProviderConfig
	if err := mapstructure.Decode(settings, &config); err != nil {
		return nil, errors.Wrap(err, "failed to decode settings")
	}
	if err := defaults.Set(&config); err != nil {
		return nil, errors.Wrap(err, "failed to set defaults")
	}
	zlog.Debug().Msgf("playlist provider config: %+v", config)
	if err := validator.New().Struct(config); err != nil {
		zlog.Error().Msgf("playlist provider validation failed: %v", err)
		return nil, errors.Wrap(err, "validation failed")
	}
	return &PlaylistProvider{
		spotify:        spotify,
		cache:          make([]track.Track, 0),
		candidateCount: candidateCount,
		config:         &config}, nil
}

// GetCandidates retrieves random tracks from the configured playlist.
// Maintains a cache to avoid redundant API calls when random selection returns duplicates.
func (p *PlaylistProvider) GetCandidates(ctx context.Context, count int, seedTracks []track.Track, existingTrackIDs map[string]bool) ([]track.Track, error) {
	if count <= 0 {
		return []track.Track{}, nil
	}

	// Filter cache to exclude tracks already in queue
	availableFromCache := make([]track.Track, 0)
	for _, t := range p.cache {
		if !existingTrackIDs[t.ID] {
			availableFromCache = append(availableFromCache, t)
		}
	}

	// If cache doesn't have enough tracks, fetch more from Spotify
	if len(availableFromCache) < count {
		needed := p.candidateCount - len(availableFromCache)
		newTracks, err := p.spotify.GetPlaylistTracksRandom(ctx, p.config.PlaylistURL, needed)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get random tracks from playlist")
		}

		// Filter out duplicates and already queued tracks
		for _, t := range newTracks {
			if !existingTrackIDs[t.ID] && !contains(availableFromCache, t.ID) {
				availableFromCache = append(availableFromCache, t)
			}
		}
	}

	// Return requested count and update cache with remaining
	if len(availableFromCache) == 0 {
		return []track.Track{}, nil
	}

	returnCount := count
	if returnCount > len(availableFromCache) {
		returnCount = len(availableFromCache)
	}

	result := availableFromCache[:returnCount]
	p.cache = availableFromCache[returnCount:]

	return result, nil
}

// Name returns the provider name.
func (p *PlaylistProvider) Name() string {
	return "playlist"
}

// contains checks if a track ID is in the slice.
func contains(tracks []track.Track, id string) bool {
	for _, t := range tracks {
		if t.ID == id {
			return true
		}
	}
	return false
}
