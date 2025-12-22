package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
	{
		name: "valid config",
		config: Config{
			Spotify: SpotifyConfig{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				RefreshToken: "test-refresh-token",
				Market:       "JP",
			},
			Admin: AdminConfig{
				Token:        "test-admin-token",
				DisplayNames: []string{"AdminUser"},
			},
			Playlists: PlaylistsConfig{
				Opening: PlaylistEntryConfig{PlaylistURL: "spotify:playlist:opening", DisplayName: "Opening"},
				Ending:  PlaylistEntryConfig{PlaylistURL: "spotify:playlist:ending", DisplayName: "Ending"},
			},
			BGM: BGMConfig{
				Providers: []ProviderConfig{
					{
						Type:        "lastfm",
						DisplayName: "Last.fm",
						Settings:    map[string]any{"api_key": "test-api-key"},
					},
				},
			},
		},
		wantErr: false,
	},
		{
			name: "missing spotify client id",
			config: Config{
				Spotify: SpotifyConfig{
					ClientSecret: "test-client-secret",
					RefreshToken: "test-refresh-token",
				},
				Admin: AdminConfig{
					Token: "test-admin-token",
				},
				Playlists: PlaylistsConfig{
					Opening: PlaylistEntryConfig{PlaylistURL: "spotify:playlist:opening", DisplayName: "Opening"},
					Ending:  PlaylistEntryConfig{PlaylistURL: "spotify:playlist:ending", DisplayName: "Ending"},
				},
			},
			wantErr: true,
			errMsg:  "ClientID",
		},
		{
			name: "missing spotify client secret",
			config: Config{
				Spotify: SpotifyConfig{
					ClientID:     "test-client-id",
					RefreshToken: "test-refresh-token",
				},
				Admin: AdminConfig{
					Token: "test-admin-token",
				},
				Playlists: PlaylistsConfig{
					Opening: PlaylistEntryConfig{PlaylistURL: "spotify:playlist:opening", DisplayName: "Opening"},
					Ending:  PlaylistEntryConfig{PlaylistURL: "spotify:playlist:ending", DisplayName: "Ending"},
				},
			},
			wantErr: true,
			errMsg:  "ClientSecret",
		},
		{
			name: "missing admin token",
			config: Config{
				Spotify: SpotifyConfig{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					RefreshToken: "test-refresh-token",
				},
				Playlists: PlaylistsConfig{
					Opening: PlaylistEntryConfig{PlaylistURL: "spotify:playlist:opening", DisplayName: "Opening"},
					Ending:  PlaylistEntryConfig{PlaylistURL: "spotify:playlist:ending", DisplayName: "Ending"},
				},
			},
			wantErr: true,
			errMsg:  "Token",
		},
		{
			name: "invalid market length",
			config: Config{
				Spotify: SpotifyConfig{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					RefreshToken: "test-refresh-token",
					Market:       "JAPAN", // 2文字ではない
				},
				Admin: AdminConfig{
					Token: "test-admin-token",
				},
				Playlists: PlaylistsConfig{
					Opening: PlaylistEntryConfig{PlaylistURL: "spotify:playlist:opening", DisplayName: "Opening"},
					Ending:  PlaylistEntryConfig{PlaylistURL: "spotify:playlist:ending", DisplayName: "Ending"},
				},
			},
			wantErr: true,
			errMsg:  "Market",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				require.Error(t, err, "expected validation to fail")
				assert.Contains(t, err.Error(), tt.errMsg,
					"error message should mention the problematic field")
			} else {
				assert.NoError(t, err, "expected validation to pass")
			}
		})
	}
}

func TestConfig_ParseStartTime(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
		wantNil   bool
		wantErr   bool
	}{
		{
			name:      "empty start time",
			startTime: "",
			wantNil:   true,
			wantErr:   false,
		},
		{
			name:      "valid RFC3339 time",
			startTime: "2024-01-01T12:00:00Z",
			wantNil:   false,
			wantErr:   false,
		},
		{
			name:      "invalid time format",
			startTime: "2024-01-01 12:00:00",
			wantNil:   false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Session: SessionConfig{
					StartTime: tt.startTime,
				},
			}

			result, err := cfg.ParseStartTime()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.wantNil {
					assert.Nil(t, result)
				} else {
					assert.NotNil(t, result)
				}
			}
		})
	}
}

func TestConfig_ParseEndTime(t *testing.T) {
	tests := []struct {
		name    string
		endTime string
		wantNil bool
		wantErr bool
	}{
		{
			name:    "empty end time",
			endTime: "",
			wantNil: true,
			wantErr: false,
		},
		{
			name:    "valid RFC3339 time",
			endTime: "2024-01-01T18:00:00Z",
			wantNil: false,
			wantErr: false,
		},
		{
			name:    "invalid time format",
			endTime: "invalid",
			wantNil: false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Session: SessionConfig{
					EndTime: tt.endTime,
				},
			}

			result, err := cfg.ParseEndTime()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.wantNil {
					assert.Nil(t, result)
				} else {
					assert.NotNil(t, result)
				}
			}
		})
	}
}
