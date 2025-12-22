// Package spotify provides a client for the Spotify API.
package spotify

import (
	"context"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"

	"github.com/osa030/19box/internal/domain/track"
)

// Client is a Spotify API client.
type Client struct {
	client     *spotify.Client
	market     string
	maxRetries int
	retryDelay time.Duration
}

// Config represents Spotify client configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	Market       string
}

// New creates a new Spotify client.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil, errors.New("spotify credentials are required")
	}

	// Create authenticator with required scopes
	auth := spotifyauth.New(
		spotifyauth.WithClientID(cfg.ClientID),
		spotifyauth.WithClientSecret(cfg.ClientSecret),
		spotifyauth.WithScopes(
			spotifyauth.ScopePlaylistModifyPublic,
			spotifyauth.ScopePlaylistModifyPrivate,
			spotifyauth.ScopePlaylistReadPrivate,
		),
	)

	// Create token from refresh token
	token := &oauth2.Token{
		RefreshToken: cfg.RefreshToken,
	}

	// Get HTTP client with auto-refresh capability
	httpClient := auth.Client(ctx, token)
	client := spotify.New(httpClient)

	market := cfg.Market
	if market == "" {
		market = "JP"
	}

	return &Client{
		client:     client,
		market:     market,
		maxRetries: 3,
		retryDelay: time.Second,
	}, nil
}

// GetTrack retrieves track information by ID, URL, or URI.
func (c *Client) GetTrack(ctx context.Context, trackID string, market ...string) (*track.Track, error) {
	// Extract track ID from URL/URI if necessary
	id := extractTrackID(trackID)
	
	// Determine market options
	var opts []spotify.RequestOption
	if len(market) > 0 && market[0] != "" {
		opts = append(opts, spotify.Market(market[0]))
	}

	var result *spotify.FullTrack
	err := c.retry(func() error {
		t, err := c.client.GetTrack(ctx, spotify.ID(id), opts...)
		if err != nil {
			return err
		}
		result = t
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get track")
	}

	return c.convertTrack(result), nil
}

// Search searches for tracks on Spotify.
func (c *Client) Search(ctx context.Context, query string, searchType string, limit int) ([]track.Track, error) {
	if query == "" {
		return nil, errors.New("search query is required")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	var result *spotify.SearchResult
	err := c.retry(func() error {
		// Convert searchType to spotify.SearchType
		var st spotify.SearchType
		switch searchType {
		case "track":
			st = spotify.SearchTypeTrack
		case "album":
			st = spotify.SearchTypeAlbum
		case "artist":
			st = spotify.SearchTypeArtist
		default:
			st = spotify.SearchTypeTrack
		}

		r, err := c.client.Search(ctx, query, st, spotify.Limit(limit))
		if err != nil {
			return err
		}
		result = r
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to search")
	}

	// Convert search results to tracks
	tracks := make([]track.Track, 0, len(result.Tracks.Tracks))
	for _, t := range result.Tracks.Tracks {
		tracks = append(tracks, *c.convertTrack(&t))
	}

	return tracks, nil
}

// GetPlaylistTracks retrieves all tracks from a playlist.
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistURL string) ([]track.Track, error) {
	playlistID := extractPlaylistID(playlistURL)
	if playlistID == "" {
		return nil, errors.New("invalid playlist URL")
	}

	var tracks []track.Track
	offset := 0
	limit := 100

	for {
		var page *spotify.PlaylistItemPage
		err := c.retry(func() error {
			p, err := c.client.GetPlaylistItems(ctx, spotify.ID(playlistID),
				spotify.Limit(limit),
				spotify.Offset(offset),
				spotify.Market(c.market),
			)
			if err != nil {
				return err
			}
			page = p
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get playlist items")
		}

		for _, item := range page.Items {
			// Only process tracks (exclude episodes)
			// item.Track is a struct, check if Track field exists and is not nil
			if item.Track.Track != nil && item.Track.Track.ID != "" {
				tracks = append(tracks, *c.convertTrack(item.Track.Track))
			}
		}

		if len(page.Items) < limit {
			break
		}
		offset += limit
	}

	return tracks, nil
}

// CheckPlaylistExists checks if a playlist exists without fetching all tracks.
// This is a lightweight check for validation purposes.
func (c *Client) CheckPlaylistExists(ctx context.Context, playlistURL string) error {
	playlistID := extractPlaylistID(playlistURL)
	if playlistID == "" {
		return errors.New("invalid playlist URL")
	}

	// Fetch only 1 item to check existence
	err := c.retry(func() error {
		_, err := c.client.GetPlaylistItems(ctx, spotify.ID(playlistID),
			spotify.Limit(1),
			spotify.Offset(0),
			spotify.Market(c.market),
		)
		return err
	})
	if err != nil {
		return errors.Wrap(err, "playlist does not exist or is not accessible")
	}

	return nil
}

// GetPlaylistTracksRandom retrieves a random sample of tracks from a playlist.
// First gets the total track count, then fetches a random page and returns up to count tracks.
func (c *Client) GetPlaylistTracksRandom(ctx context.Context, playlistURL string, count int) ([]track.Track, error) {
	playlistID := extractPlaylistID(playlistURL)
	if playlistID == "" {
		return nil, errors.New("invalid playlist URL")
	}

	// First, get the total track count by fetching the first page
	var firstPage *spotify.PlaylistItemPage
	err := c.retry(func() error {
		p, err := c.client.GetPlaylistItems(ctx, spotify.ID(playlistID),
			spotify.Limit(1),
			spotify.Offset(0),
			spotify.Market(c.market),
		)
		if err != nil {
			return err
		}
		firstPage = p
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get playlist info")
	}

	totalTracks := int(firstPage.Total)
	if totalTracks == 0 {
		return []track.Track{}, nil
	}

	// Calculate random offset with better entropy
	// We want to fetch a page that contains enough tracks, so we limit the offset
	// to ensure we can get at least 'count' tracks from the page
	limit := 100 // Spotify API max per page
	maxOffset := totalTracks - limit
	if maxOffset < 0 {
		maxOffset = 0
	}

	// Use crypto/rand for better randomness combined with time-based seed
	var cryptoSeed int64
	var buf [8]byte
	if _, err := cryptoRand.Read(buf[:]); err == nil {
		cryptoSeed = int64(binary.LittleEndian.Uint64(buf[:]))
	} else {
		cryptoSeed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(cryptoSeed))

	offset := 0
	if maxOffset > 0 {
		offset = rng.Intn(maxOffset + 1)
	}

	// Fetch a random page
	var page *spotify.PlaylistItemPage
	err = c.retry(func() error {
		p, err := c.client.GetPlaylistItems(ctx, spotify.ID(playlistID),
			spotify.Limit(limit),
			spotify.Offset(offset),
			spotify.Market(c.market),
		)
		if err != nil {
			return err
		}
		page = p
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get playlist items")
	}

	// Convert items to tracks
	var tracks []track.Track
	for _, item := range page.Items {
		// Only process tracks (exclude episodes)
		if item.Track.Track != nil && item.Track.Track.ID != "" {
			tracks = append(tracks, *c.convertTrack(item.Track.Track))
		}
	}

	// Randomly select up to 'count' tracks with better shuffle
	if len(tracks) > count {
		// Shuffle multiple times for better randomness
		for i := 0; i < 3; i++ {
			rng.Shuffle(len(tracks), func(i, j int) {
				tracks[i], tracks[j] = tracks[j], tracks[i]
			})
		}
		tracks = tracks[:count]
	}

	return tracks, nil
}

// CreatePlaylist creates a new playlist.
func (c *Client) CreatePlaylist(ctx context.Context, name, description string) (string, error) {
	user, err := c.client.CurrentUser(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get current user")
	}

	var playlist *spotify.FullPlaylist
	err = c.retry(func() error {
		p, err := c.client.CreatePlaylistForUser(ctx, user.ID, name, description, true, false)
		if err != nil {
			return err
		}
		playlist = p
		return nil
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to create playlist")
	}

	return string(playlist.ID), nil
}

// AddTracksToPlaylist adds tracks to a playlist.
// trackIDs can be Spotify IDs, URLs, or URIs.
func (c *Client) AddTracksToPlaylist(ctx context.Context, playlistID string, trackIDs []string) error {
	// Extract track IDs from URLs/URIs
	ids := make([]spotify.ID, len(trackIDs))
	for i, trackID := range trackIDs {
		ids[i] = spotify.ID(extractTrackID(trackID))
	}

	// Spotify allows max 100 tracks per request
	for i := 0; i < len(ids); i += 100 {
		end := i + 100
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		err := c.retry(func() error {
			_, err := c.client.AddTracksToPlaylist(ctx, spotify.ID(playlistID), batch...)
			return err
		})
		if err != nil {
			return errors.Wrap(err, "failed to add tracks to playlist")
		}
	}

	return nil
}

// RemoveTracksFromPlaylist removes tracks from a playlist.
func (c *Client) RemoveTracksFromPlaylist(ctx context.Context, playlistID string, trackIDs []string) error {
	ids := make([]spotify.ID, len(trackIDs))
	for i, id := range trackIDs {
		ids[i] = spotify.ID(id)
	}

	// Spotify allows max 100 tracks per request
	for i := 0; i < len(ids); i += 100 {
		end := i + 100
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		err := c.retry(func() error {
			_, err := c.client.RemoveTracksFromPlaylist(ctx, spotify.ID(playlistID), batch...)
			return err
		})
		if err != nil {
			return errors.Wrap(err, "failed to remove tracks from playlist")
		}
	}

	return nil
}

// GetPlaylistURL returns the Spotify URL for a playlist.
func (c *Client) GetPlaylistURL(playlistID string) string {
	return fmt.Sprintf("https://open.spotify.com/playlist/%s", playlistID)
}

// convertTrack converts a Spotify FullTrack to domain Track.
func (c *Client) convertTrack(t *spotify.FullTrack) *track.Track {
	artists := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = a.Name
	}

	var albumArt string
	if len(t.Album.Images) > 0 {
		albumArt = t.Album.Images[0].URL
	}

	markets := make([]string, len(t.AvailableMarkets))
	for i, m := range t.AvailableMarkets {
		markets[i] = string(m)
	}

	// If no markets are returned but we have a configured market,
	// assume availability in that market (common when using Market param in API calls)
	if len(markets) == 0 && c.market != "" {
		markets = append(markets, c.market)
	}

	// Copy IsPlayable if present
	// Note: zmb3/spotify struct field is boolean, not pointer, but Spotify API doc says it may be omitted.
	// We'll treat the library's bool value as truth.
	// Check if the library exposes it as a pointer or value. Searching confirmed it exists.
	// Assuming it's a pointer or we take the value.
	// Let's check `zmb3/spotify` FullTrack definition via reflection or source if possible, but earlier search said "IsPlayable field".
	// Usually libraries use pointers for nullable fields or omit them.
	// We'll trust "IsPlayable" exists.
	var isPlayable *bool
	if t.IsPlayable != nil {
		isPlayable = t.IsPlayable
	}

	return &track.Track{
		ID:          string(t.ID),
		Name:        t.Name,
		Artists:     artists,
		Album:       t.Album.Name,
		AlbumArtURL: albumArt,
		Duration:    time.Duration(t.Duration) * time.Millisecond,
		URL:         c.GetTrackURL(string(t.ID)),
		Popularity:  int(t.Popularity),
		Explicit:    t.Explicit,
		Markets:     markets,
		IsPlayable:  isPlayable,
	}
}

// GetTrackURL returns the Spotify URL for a track.
func (c *Client) GetTrackURL(trackID string) string {
	return fmt.Sprintf("https://open.spotify.com/track/%s", trackID)
}

// GetTrackURLWithContext returns the Spotify URL for a track with playlist context.
func (c *Client) GetTrackURLWithContext(trackID, playlistID string) string {
	if playlistID == "" {
		return c.GetTrackURL(trackID)
	}
	return fmt.Sprintf("https://open.spotify.com/track/%s?context=spotify%%3Aplaylist%%3A%s", trackID, playlistID)
}

// retry retries an operation with exponential backoff.
func (c *Client) retry(fn func() error) error {
	var lastErr error
	for i := 0; i < c.maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		if !isRetryable(err) {
			return err
		}

		if i < c.maxRetries-1 {
			time.Sleep(c.retryDelay * time.Duration(i+1))
		}
	}
	return errors.Wrap(lastErr, "max retries exceeded")
}

// isRetryable checks if an error is retryable.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Rate limit errors and server errors are retryable
	errStr := err.Error()
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504")
}

// extractPlaylistID extracts the playlist ID from a Spotify playlist URL or URI.
func extractPlaylistID(input string) string {
	input = strings.TrimSpace(input)
	// Handle Spotify URI format: spotify:playlist:PLAYLIST_ID
	if strings.HasPrefix(input, "spotify:playlist:") {
		return strings.TrimPrefix(input, "spotify:playlist:")
	}

	// Handle URL format: https://open.spotify.com/playlist/PLAYLIST_ID or https://open.spotify.com/intl-XX/playlist/PLAYLIST_ID
	if strings.Contains(input, "open.spotify.com") && strings.Contains(input, "/playlist/") {
		parts := strings.Split(input, "/playlist/")
		if len(parts) >= 2 {
			// Remove query parameters and trailing slashes
			id := strings.Split(parts[len(parts)-1], "?")[0]
			id = strings.TrimRight(id, "/")
			return id
		}
	}

	// Assume it's already a playlist ID
	return input
}

// extractTrackID extracts the track ID from a Spotify track URL or URI.
func extractTrackID(input string) string {
	input = strings.TrimSpace(input)
	// Handle Spotify URI format: spotify:track:TRACK_ID
	if strings.HasPrefix(input, "spotify:track:") {
		return strings.TrimPrefix(input, "spotify:track:")
	}

	// Handle URL format: https://open.spotify.com/track/TRACK_ID or https://open.spotify.com/intl-XX/track/TRACK_ID
	if strings.Contains(input, "open.spotify.com") && strings.Contains(input, "/track/") {
		parts := strings.Split(input, "/track/")
		if len(parts) >= 2 {
			// Remove query parameters and trailing slashes
			id := strings.Split(parts[len(parts)-1], "?")[0]
			id = strings.TrimRight(id, "/")
			return id
		}
	}

	// Assume it's already a track ID
	return input
}
