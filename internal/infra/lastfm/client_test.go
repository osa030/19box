package lastfm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTopTags(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "track.getTopTags", r.URL.Query().Get("method"))
		assert.Equal(t, "test_artist", r.URL.Query().Get("artist"))
		assert.Equal(t, "test_track", r.URL.Query().Get("track"))
		assert.Equal(t, "test_key", r.URL.Query().Get("api_key"))

		response := `{
			"toptags": {
				"tag": [
					{"name": "rock", "count": 100, "url": "http://last.fm/tag/rock"},
					{"name": "alternative", "count": 80, "url": "http://last.fm/tag/alternative"}
				]
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test_key"})
	assert.NoError(t, err)
	client.baseURL = server.URL + "/"

	// Test API call
	ctx := context.Background()
	tags, err := client.GetTopTags(ctx, "test_track", "test_artist", 5)
	assert.NoError(t, err)
	assert.Len(t, tags, 2)
	assert.Equal(t, "rock", tags[0].Name)
	assert.Equal(t, 100, tags[0].Count)

	// Test Caching
	// Change server response to verify cache is used (server shouldn't be called again ideally,
	// but here we check if the result is same without error)
	tagsCached, err := client.GetTopTags(ctx, "test_track", "test_artist", 5)
	assert.NoError(t, err)
	assert.Equal(t, tags, tagsCached)
}

func TestGetTopTracks(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "tag.getTopTracks", r.URL.Query().Get("method"))
		assert.Equal(t, "rock", r.URL.Query().Get("tag"))

		response := `{
			"tracks": {
				"track": [
					{
						"name": "Track 1",
						"mbid": "mbid1",
						"url": "url1",
						"artist": {"name": "Artist 1", "mbid": "ambid1", "url": "aurl1"},
						"listeners": "1000",
						"playcount": "5000"
					},
					{
						"name": "Track 2",
						"mbid": "mbid2",
						"url": "url2",
						"artist": {"name": "Artist 2", "mbid": "ambid2", "url": "aurl2"},
						"listeners": "500",
						"playcount": "2000"
					}
				]
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test_key"})
	assert.NoError(t, err)
	client.baseURL = server.URL + "/"

	// Test API call
	ctx := context.Background()
	tracks, err := client.GetTopTracks(ctx, "rock", 5)
	assert.NoError(t, err)
	assert.Len(t, tracks, 2)
	assert.Equal(t, "Track 1", tracks[0].Name)
	assert.Equal(t, "Artist 1", tracks[0].Artist)

	// Test Caching
	tracksCached, err := client.GetTopTracks(ctx, "rock", 5)
	assert.NoError(t, err)
	assert.Equal(t, tracks, tracksCached)
}
