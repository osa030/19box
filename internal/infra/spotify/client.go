// Package spotify provides a client for the Spotify API.
package spotify

import (
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"golang.org/x/oauth2"

	"github.com/osa030/19box/internal/domain/track"
)

// --- Constants ---

const (
	spotifyAPIBaseURL      = "https://api.spotify.com/v1/"
	spotifyTokenURL        = "https://accounts.spotify.com/api/token"
	spotifyMaxItemsPerBatch = 100

	defaultMarket      = "JP"
	defaultMaxRetries  = 3
	defaultRetryDelay  = time.Second
	defaultSearchLimit = 5
	maxSearchLimit     = 10
)

// Client is a Spotify API client.
type Client struct {
	httpClient *http.Client
	baseURL    string
	market     string
	maxRetries int
	retryDelay time.Duration
}

// New creates a new Spotify client.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil, errors.New("spotify credentials are required")
	}

	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL: spotifyTokenURL,
		},
	}
	token := &oauth2.Token{RefreshToken: cfg.RefreshToken}
	httpClient := oauthCfg.Client(ctx, token)

	market := cfg.Market
	if market == "" {
		market = defaultMarket
	}

	return &Client{
		httpClient: httpClient,
		baseURL:    spotifyAPIBaseURL,
		market:     market,
		maxRetries: defaultMaxRetries,
		retryDelay: defaultRetryDelay,
	}, nil
}

// doRequest executes an HTTP request and decodes the JSON response into result (if non-nil).
func (c *Client) doRequest(ctx context.Context, method, rawURL string, body io.Reader, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		apiErr := &spotifyAPIError{StatusCode: resp.StatusCode}
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if decErr := json.NewDecoder(resp.Body).Decode(&errBody); decErr == nil {
			apiErr.Message = errBody.Error.Message
		}
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(resp.StatusCode)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, parseErr := strconv.Atoi(ra); parseErr == nil {
					apiErr.RetryAfter = time.Duration(secs) * time.Second
				}
			}
		}
		return apiErr
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// GetTrack retrieves track information by ID, URL, or URI.
// https://developer.spotify.com/documentation/web-api/reference/get-track
func (c *Client) GetTrack(ctx context.Context, trackID string, market ...string) (*track.Track, error) {
	id := extractTrackID(trackID)
	m := c.market
	if len(market) > 0 && market[0] != "" {
		m = market[0]
	}

	params := url.Values{"market": {m}}
	rawURL := c.baseURL + "tracks/" + url.PathEscape(id) + "?" + params.Encode()

	var result apiTrack
	err := c.retry(func() error {
		return c.doRequest(ctx, http.MethodGet, rawURL, nil, &result)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get track")
	}

	return c.convertTrack(&result), nil
}

// Search searches for tracks on Spotify.
// https://developer.spotify.com/documentation/web-api/reference/search
func (c *Client) Search(ctx context.Context, query string, searchType string, limit int) ([]track.Track, error) {
	if query == "" {
		return nil, errors.New("search query is required")
	}

	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	if searchType == "" {
		searchType = "track"
	}

	params := url.Values{
		"q":     {query},
		"type":  {searchType},
		"limit": {strconv.Itoa(limit)},
	}
	rawURL := c.baseURL + "search?" + params.Encode()

	var result apiSearchResult
	err := c.retry(func() error {
		return c.doRequest(ctx, http.MethodGet, rawURL, nil, &result)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to search")
	}

	tracks := make([]track.Track, 0, len(result.Tracks.Items))
	for i := range result.Tracks.Items {
		tracks = append(tracks, *c.convertTrack(&result.Tracks.Items[i]))
	}

	return tracks, nil
}

// getPlaylistItemPage fetches a single page of playlist items.
// https://developer.spotify.com/documentation/web-api/reference/get-playlists-items
func (c *Client) getPlaylistItemPage(ctx context.Context, playlistID string, limit, offset int) (*apiPlaylistItemPage, error) {
	params := url.Values{
		"limit":  {strconv.Itoa(limit)},
		"offset": {strconv.Itoa(offset)},
		"market": {c.market},
	}
	rawURL := c.baseURL + "playlists/" + url.PathEscape(playlistID) + "/items?" + params.Encode()

	var page apiPlaylistItemPage
	err := c.retry(func() error {
		return c.doRequest(ctx, http.MethodGet, rawURL, nil, &page)
	})
	if err != nil {
		return nil, err
	}
	return &page, nil
}

// GetPlaylistTracks retrieves all tracks from a playlist.
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistURL string) ([]track.Track, error) {
	playlistID := extractPlaylistID(playlistURL)
	if playlistID == "" {
		return nil, errors.New("invalid playlist URL")
	}

	var tracks []track.Track
	offset := 0
	limit := spotifyMaxItemsPerBatch

	for {
		page, err := c.getPlaylistItemPage(ctx, playlistID, limit, offset)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get playlist items")
		}

		for i := range page.Items {
			item := &page.Items[i]
			if item.Track != nil && item.Track.Type == "track" && item.Track.ID != "" {
				tracks = append(tracks, *c.convertTrack(&item.Track.apiTrack))
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

	_, err := c.getPlaylistItemPage(ctx, playlistID, 1, 0)
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

	// Get total track count by fetching the first page
	firstPage, err := c.getPlaylistItemPage(ctx, playlistID, 1, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get playlist info")
	}

	totalTracks := firstPage.Total
	if totalTracks == 0 {
		return []track.Track{}, nil
	}

	// Calculate random offset, ensuring we can get at least 'count' tracks from the page
	limit := spotifyMaxItemsPerBatch
	maxOffset := totalTracks - limit
	if maxOffset < 0 {
		maxOffset = 0
	}

	// Use crypto/rand for better randomness combined with time-based fallback
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

	page, err := c.getPlaylistItemPage(ctx, playlistID, limit, offset)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get playlist items")
	}

	var tracks []track.Track
	for i := range page.Items {
		item := &page.Items[i]
		if item.Track != nil && item.Track.Type == "track" && item.Track.ID != "" {
			tracks = append(tracks, *c.convertTrack(&item.Track.apiTrack))
		}
	}

	// Randomly select up to 'count' tracks with better shuffle
	if len(tracks) > count {
		for i := 0; i < 3; i++ {
			rng.Shuffle(len(tracks), func(i, j int) {
				tracks[i], tracks[j] = tracks[j], tracks[i]
			})
		}
		tracks = tracks[:count]
	}

	return tracks, nil
}

// CreatePlaylist creates a new playlist using /me/playlists.
// https://developer.spotify.com/documentation/web-api/reference/create-playlist
func (c *Client) CreatePlaylist(ctx context.Context, name, description string) (string, error) {
	rawURL := c.baseURL + "me/playlists"
	body, err := json.Marshal(map[string]any{
		"name":        name,
		"description": description,
		"public":      true,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to encode playlist request")
	}

	var playlist apiPlaylist
	err = c.retry(func() error {
		return c.doRequest(ctx, http.MethodPost, rawURL, bytes.NewReader(body), &playlist)
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to create playlist")
	}

	return playlist.ID, nil
}

// AddTracksToPlaylist adds tracks to a playlist.
// trackIDs can be Spotify IDs, URLs, or URIs.
// hhttps://developer.spotify.com/documentation/web-api/reference/add-items-to-playlist
func (c *Client) AddTracksToPlaylist(ctx context.Context, playlistID string, trackIDs []string) error {
	uris := make([]string, len(trackIDs))
	for i, id := range trackIDs {
		uris[i] = "spotify:track:" + extractTrackID(id)
	}

	rawURL := c.baseURL + "playlists/" + url.PathEscape(playlistID) + "/items"

	// Spotify allows max 100 tracks per request
	for i := 0; i < len(uris); i += spotifyMaxItemsPerBatch {
		end := i + spotifyMaxItemsPerBatch
		if end > len(uris) {
			end = len(uris)
		}
		batch := uris[i:end]

		body, err := json.Marshal(map[string]any{"uris": batch})
		if err != nil {
			return errors.Wrap(err, "failed to encode request")
		}

		err = c.retry(func() error {
			return c.doRequest(ctx, http.MethodPost, rawURL, bytes.NewReader(body), nil)
		})
		if err != nil {
			return errors.Wrap(err, "failed to add tracks to playlist")
		}
	}

	return nil
}

// RemoveTracksFromPlaylist removes tracks from a playlist.
// https://developer.spotify.com/documentation/web-api/reference/remove-items-playlist
func (c *Client) RemoveTracksFromPlaylist(ctx context.Context, playlistID string, trackIDs []string) error {
	rawURL := c.baseURL + "playlists/" + url.PathEscape(playlistID) + "/items"

	// Spotify allows max 100 tracks per request
	for i := 0; i < len(trackIDs); i += spotifyMaxItemsPerBatch {
		end := i + spotifyMaxItemsPerBatch
		if end > len(trackIDs) {
			end = len(trackIDs)
		}
		batch := trackIDs[i:end]

		type itemURI struct {
			URI string `json:"uri"`
		}
		items := make([]itemURI, len(batch))
		for j, id := range batch {
			items[j] = itemURI{URI: "spotify:track:" + id}
		}

		body, err := json.Marshal(map[string]any{"items": items})
		if err != nil {
			return errors.Wrap(err, "failed to encode request")
		}

		err = c.retry(func() error {
			return c.doRequest(ctx, http.MethodDelete, rawURL, bytes.NewReader(body), nil)
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

// convertTrack converts an internal API track to a domain Track.
func (c *Client) convertTrack(t *apiTrack) *track.Track {
	artists := make([]string, len(t.Artists))
	artistIDs := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = a.Name
		artistIDs[i] = a.ID
	}

	var albumArt string
	if len(t.Album.Images) > 0 {
		albumArt = t.Album.Images[0].URL
	}

	markets := make([]string, len(t.AvailableMarkets))
	copy(markets, t.AvailableMarkets)

	// If no markets are returned but we have a configured market,
	// assume availability in that market (common when using Market param in API calls)
	if len(markets) == 0 && c.market != "" {
		markets = append(markets, c.market)
	}

	return &track.Track{
		ID:          t.ID,
		Name:        t.Name,
		Artists:     artists,
		ArtistIDs:   artistIDs,
		Album:       t.Album.Name,
		AlbumArtURL: albumArt,
		Duration:    time.Duration(t.DurationMs) * time.Millisecond,
		URL:         c.GetTrackURL(t.ID),
		Popularity:  t.Popularity,
		Explicit:    t.Explicit,
		Markets:     markets,
		IsPlayable:  t.IsPlayable,
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

// retry retries an operation with backoff, honoring Retry-After headers on 429.
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

		delay := c.retryDelay * time.Duration(i+1)
		var apiErr *spotifyAPIError
		if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
			delay = apiErr.RetryAfter
		}

		if i < c.maxRetries-1 {
			time.Sleep(delay)
		}
	}
	return errors.Wrap(lastErr, "max retries exceeded")
}

// isRetryable checks if an error should trigger a retry.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *spotifyAPIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests ||
			(apiErr.StatusCode >= 500 && apiErr.StatusCode < 600)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
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
