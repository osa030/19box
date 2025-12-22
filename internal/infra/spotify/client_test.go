package spotify

import (
	"errors"
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
			name:     "rate limit error with 429",
			err:      errors.New("Error 429: rate limit exceeded"),
			expected: true,
		},
		{
			name:     "rate limit text",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "server error 500",
			err:      errors.New("Error 500: internal server error"),
			expected: true,
		},
		{
			name:     "server error 502",
			err:      errors.New("502 Bad Gateway"),
			expected: true,
		},
		{
			name:     "server error 503",
			err:      errors.New("503 Service Unavailable"),
			expected: true,
		},
		{
			name:     "server error 504",
			err:      errors.New("504 Gateway Timeout"),
			expected: true,
		},
		{
			name:     "client error 400",
			err:      errors.New("400 Bad Request"),
			expected: false,
		},
		{
			name:     "not found error",
			err:      errors.New("404 not found"),
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
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
