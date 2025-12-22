// Package lastfm provides a client for the Last.fm API.
package lastfm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	zlog "github.com/rs/zerolog/log"
)

// trackTagCacheEntry represents a cached track tag result.
type trackTagCacheEntry struct {
	tags []Tag
}

// tagTracksCacheEntry represents a cached tag tracks result.
type tagTracksCacheEntry struct {
	tracks []TopTrack
}

// Client is a Last.fm API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client

	// Cache for track tags
	trackTagCache map[string]*trackTagCacheEntry
	// Cache for tag tracks
	tagTracksCache map[string]*tagTracksCacheEntry

	// Mutex for cache access
	cacheMu sync.RWMutex
}

// Config represents Last.fm client configuration.
type Config struct {
	APIKey string
}

// SimilarTrack represents a similar track from Last.fm.
type SimilarTrack struct {
	Name   string
	Artist string
}

// Tag represents a Last.fm tag.
type Tag struct {
	Name  string
	Count int // Tag count/frequency
}

// TopTrack represents a top track for a tag.
type TopTrack struct {
	Name   string
	Artist string
}

// GetSimilarResponse represents the response from track.getSimilar API.
type GetSimilarResponse struct {
	SimilarTracks struct {
		Track []struct {
			Name   string `json:"name"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"track"`
	} `json:"similartracks"`
}

// GetTopTagsResponse represents the response from track.getTopTags API.
type GetTopTagsResponse struct {
	TopTags struct {
		Tag []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"tag"`
	} `json:"toptags"`
}

// GetTopTracksResponse represents the response from tag.getTopTracks API.
type GetTopTracksResponse struct {
	Tracks struct {
		Track []struct {
			Name   string `json:"name"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"track"`
	} `json:"tracks"`
}

// LastFMError represents an error response from Last.fm API.
type LastFMError struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
}

// New creates a new Last.fm client.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("last.fm API key is required")
	}

	return &Client{
		apiKey:         cfg.APIKey,
		baseURL:        "https://ws.audioscrobbler.com/2.0/",
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		trackTagCache:  make(map[string]*trackTagCacheEntry),
		tagTracksCache: make(map[string]*tagTracksCacheEntry),
	}, nil
}

// GetSimilarTracks retrieves similar tracks from Last.fm based on track name and artist.
// Reference: https://www.last.fm/api/show/track.getSimilar
func (c *Client) GetSimilarTracks(ctx context.Context, trackName, artistName string, limit int) ([]SimilarTrack, error) {
	if trackName == "" || artistName == "" {
		return nil, errors.New("track name and artist name are required")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	params := url.Values{}
	params.Set("method", "track.getSimilar")
	params.Set("api_key", c.apiKey)
	params.Set("artist", artistName)
	params.Set("track", trackName)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("format", "json")
	params.Set("autocorrect", "1")

	reqURL := c.baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	// Check for Last.fm API errors
	var apiError LastFMError
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error != 0 {
		return nil, errors.Errorf("last.fm API error %d: %s", apiError.Error, apiError.Message)
	}

	// Parse successful response
	var response GetSimilarResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	similarTracks := make([]SimilarTrack, 0, len(response.SimilarTracks.Track))
	for _, t := range response.SimilarTracks.Track {
		similarTracks = append(similarTracks, SimilarTrack{
			Name:   t.Name,
			Artist: t.Artist.Name,
		})
	}

	return similarTracks, nil
}

// GetTopTags retrieves top tags for a track from Last.fm.
// Reference: https://www.last.fm/api/show/track.getTopTags
func (c *Client) GetTopTags(ctx context.Context, trackName, artistName string, limit int) ([]Tag, error) {
	if trackName == "" || artistName == "" {
		return nil, errors.New("track name and artist name are required")
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Check cache first
	cacheKey := fmt.Sprintf("tracktag:%s:%s", artistName, trackName)
	c.cacheMu.RLock()
	if entry, ok := c.trackTagCache[cacheKey]; ok {
		c.cacheMu.RUnlock()
		zlog.Debug().Msgf("using cached tags for track: %s - %s", artistName, trackName)
		return entry.tags, nil
	}
	c.cacheMu.RUnlock()

	params := url.Values{}
	params.Set("method", "track.getTopTags")
	params.Set("api_key", c.apiKey)
	params.Set("artist", artistName)
	params.Set("track", trackName)
	params.Set("format", "json")
	params.Set("autocorrect", "1")

	reqURL := c.baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	// Check for Last.fm API errors
	var apiError LastFMError
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error != 0 {
		return nil, errors.Errorf("last.fm API error %d: %s", apiError.Error, apiError.Message)
	}

	// Parse successful response
	var response GetTopTagsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	tags := make([]Tag, 0, len(response.TopTags.Tag))
	for i, t := range response.TopTags.Tag {
		if i >= limit {
			break
		}
		tags = append(tags, Tag{
			Name:  t.Name,
			Count: t.Count,
		})
	}

	// Cache the result
	c.cacheMu.Lock()
	c.trackTagCache[cacheKey] = &trackTagCacheEntry{
		tags: tags,
	}
	c.cacheMu.Unlock()
	zlog.Debug().Msgf("cached tags for track: %s - %s (count: %d)", artistName, trackName, len(tags))

	return tags, nil
}

// GetTopTracks retrieves top tracks for a tag from Last.fm.
// Reference: https://www.last.fm/api/show/tag.getTopTracks
func (c *Client) GetTopTracks(ctx context.Context, tagName string, limit int) ([]TopTrack, error) {
	if tagName == "" {
		return nil, errors.New("tag name is required")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Check cache first
	cacheKey := fmt.Sprintf("tagtracks:%s", tagName)
	c.cacheMu.RLock()
	if entry, ok := c.tagTracksCache[cacheKey]; ok {
		c.cacheMu.RUnlock()
		zlog.Debug().Msgf("using cached top tracks for tag: %s", tagName)
		return entry.tracks, nil
	}
	c.cacheMu.RUnlock()

	params := url.Values{}
	params.Set("method", "tag.getTopTracks")
	params.Set("api_key", c.apiKey)
	params.Set("tag", tagName)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("format", "json")

	reqURL := c.baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	// Check for Last.fm API errors
	var apiError LastFMError
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error != 0 {
		return nil, errors.Errorf("last.fm API error %d: %s", apiError.Error, apiError.Message)
	}

	// Parse successful response
	var response GetTopTracksResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	tracks := make([]TopTrack, 0, len(response.Tracks.Track))
	for _, t := range response.Tracks.Track {
		tracks = append(tracks, TopTrack{
			Name:   t.Name,
			Artist: t.Artist.Name,
		})
	}

	// Cache the result
	c.cacheMu.Lock()
	c.tagTracksCache[cacheKey] = &tagTracksCacheEntry{
		tracks: tracks,
	}
	c.cacheMu.Unlock()
	zlog.Debug().Msgf("cached top tracks for tag: %s (count: %d)", tagName, len(tracks))

	return tracks, nil
}

// GetChartTopTracks retrieves global top tracks from Last.fm charts.
// Reference: https://www.last.fm/api/show/chart.getTopTracks
func (c *Client) GetChartTopTracks(ctx context.Context, limit int) ([]TopTrack, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	params := url.Values{}
	params.Set("method", "chart.getTopTracks")
	params.Set("api_key", c.apiKey)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("format", "json")

	reqURL := c.baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	// Check for Last.fm API errors
	var apiError LastFMError
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error != 0 {
		return nil, errors.Errorf("last.fm API error %d: %s", apiError.Error, apiError.Message)
	}

	// Parse successful response (same structure as tag.getTopTracks)
	var response GetTopTracksResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	tracks := make([]TopTrack, 0, len(response.Tracks.Track))
	for _, t := range response.Tracks.Track {
		tracks = append(tracks, TopTrack{
			Name:   t.Name,
			Artist: t.Artist.Name,
		})
	}

	return tracks, nil
}
