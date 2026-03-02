package spotify

import (
	"fmt"
	"time"
)

// --- Internal API response types ---

type apiImage struct {
	URL string `json:"url"`
}

type apiArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type apiAlbum struct {
	Name   string     `json:"name"`
	Images []apiImage `json:"images"`
}

type apiTrack struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Artists          []apiArtist `json:"artists"`
	Album            apiAlbum    `json:"album"`
	DurationMs       int         `json:"duration_ms"`
	Explicit         bool        `json:"explicit"`
	Popularity       int         `json:"popularity"`        // May be 0 after Feb 2026 API change
	AvailableMarkets []string    `json:"available_markets"` // May be empty after Feb 2026 API change
	IsPlayable       *bool       `json:"is_playable"`
}

type apiPlaylistItem struct {
	Track *apiPlaylistTrack `json:"track"`
}

type apiPlaylistTrack struct {
	Type string `json:"type"` // "track" or "episode"
	apiTrack
}

type apiPlaylistItemPage struct {
	Total int               `json:"total"`
	Items []apiPlaylistItem `json:"items"`
}

type apiSearchResult struct {
	Tracks struct {
		Items []apiTrack `json:"items"`
	} `json:"tracks"`
}

type apiPlaylist struct {
	ID string `json:"id"`
}

// --- Config ---

// Config represents Spotify client configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	Market       string
}

// --- Error type ---

type spotifyAPIError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration // set from Retry-After header on 429
}

func (e *spotifyAPIError) Error() string {
	return fmt.Sprintf("spotify API error %d: %s", e.StatusCode, e.Message)
}
