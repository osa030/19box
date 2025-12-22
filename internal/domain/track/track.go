// Package track provides the Track domain entity.
package track

import "time"

// Track represents a Spotify track entity.
// Contains only information retrieved from Spotify API.
type Track struct {
	ID          string        // Spotify Track ID
	Name        string        // Track name
	Artists     []string      // Artist names
	Album       string        // Album name
	AlbumArtURL string        // Album art URL
	Duration    time.Duration // Track duration
	URL         string        // Spotify URL
	Genres      []string      // Genres (from artist info)
	Popularity  int           // Popularity score (0-100)
	Explicit    bool          // Explicit content flag
	Markets     []string      // Available markets
	IsPlayable  *bool         // Playable in the specified market (nil if market not specified)
}

// RequesterType represents the type of requester.
type RequesterType string

const (
	RequesterTypeUser    RequesterType = "USER"
	RequesterTypeSystem  RequesterType = "SYSTEM"
	RequesterTypeOpening RequesterType = "OPENING"
	RequesterTypeEnding  RequesterType = "ENDING"
	RequesterTypeBGM     RequesterType = "BGM"
)

// Requester represents the person who requested the track.
type Requester struct {
	ID             string        // Participant UUID
	Name           string        // Display name
	ExternalUserID string        // External user ID (for bot integration, optional)
	Type           RequesterType // Type of requester
}

// QueuedTrack represents a track in the playback queue.
type QueuedTrack struct {
	Track     Track     // Spotify track info
	Requester Requester // Requester info
	AddedAt   time.Time // Time when added to queue
}

// IsAvailableInMarket checks if the track is available in the specified market.
func (t *Track) IsAvailableInMarket(market string) bool {
	// If IsPlayable is set, it takes precedence (Track Relinking support)
	if t.IsPlayable != nil {
		return *t.IsPlayable
	}

	// Fallback to checking markets list
	for _, m := range t.Markets {
		if m == market {
			return true
		}
	}
	return false
}
