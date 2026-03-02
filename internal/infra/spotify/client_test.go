package spotify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractPlaylistID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Spotify URI format",
			input:    "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
			expected: "37i9dQZF1DXcBWIGoYBM5M",
		},
		{
			name:     "Spotify URL format",
			input:    "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
			expected: "37i9dQZF1DXcBWIGoYBM5M",
		},
		{
			name:     "Spotify URL with query params",
			input:    "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M?si=abc123",
			expected: "37i9dQZF1DXcBWIGoYBM5M",
		},
		{
			name:     "Plain playlist ID",
			input:    "37i9dQZF1DXcBWIGoYBM5M",
			expected: "37i9dQZF1DXcBWIGoYBM5M",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "HTTP URL (not HTTPS)",
			input:    "http://open.spotify.com/playlist/testID",
			expected: "testID",
		},
		{
			name:     "URL with multiple query params",
			input:    "https://open.spotify.com/playlist/abc123?si=xyz&utm_source=copy",
			expected: "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPlaylistID(tt.input)
			assert.Equal(t, tt.expected, result,
				"extractPlaylistID(%s) should return %s", tt.input, tt.expected)
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "rate limit 429",
			err:      &spotifyAPIError{StatusCode: 429, Message: "rate limit exceeded"},
			expected: true,
		},
		{
			name:     "server error 500",
			err:      &spotifyAPIError{StatusCode: 500, Message: "internal server error"},
			expected: true,
		},
		{
			name:     "server error 502",
			err:      &spotifyAPIError{StatusCode: 502, Message: "bad gateway"},
			expected: true,
		},
		{
			name:     "server error 503",
			err:      &spotifyAPIError{StatusCode: 503, Message: "service unavailable"},
			expected: true,
		},
		{
			name:     "server error 504",
			err:      &spotifyAPIError{StatusCode: 504, Message: "gateway timeout"},
			expected: true,
		},
		{
			name:     "client error 400",
			err:      &spotifyAPIError{StatusCode: 400, Message: "bad request"},
			expected: false,
		},
		{
			name:     "not found 404",
			err:      &spotifyAPIError{StatusCode: 404, Message: "not found"},
			expected: false,
		},
		{
			name:     "unauthorized 401",
			err:      &spotifyAPIError{StatusCode: 401, Message: "unauthorized"},
			expected: false,
		},
		{
			name:     "generic non-api error",
			err:      assert.AnError,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
