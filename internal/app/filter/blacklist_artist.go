package filter

import (
	"context"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
	"github.com/osa030/19box/internal/infra/blacklist"
)

// BlacklistArtistFilter checks if the artist is blacklisted.
type BlacklistArtistFilter struct {
	blacklist *blacklist.Blacklist
}

// NewBlacklistArtistFilter creates a new blacklist artist filter.
func NewBlacklistArtistFilter(blacklist *blacklist.Blacklist) *BlacklistArtistFilter {
	return &BlacklistArtistFilter{
		blacklist: blacklist,
	}
}

// Name returns the filter name.
func (f *BlacklistArtistFilter) Name() string {
	return "blacklist_artist_filter"
}

// Description returns the filter description.
func (f *BlacklistArtistFilter) Description() string {
	return "Rejects track requests if any artist of the track is in the blacklist"
}

// ReturnCodes returns possible return codes.
func (f *BlacklistArtistFilter) ReturnCodes() []string {
	return []string{"blacklisted_artist"}
}

// AppliesTo returns which requester types this filter applies to.
func (f *BlacklistArtistFilter) AppliesTo(requesterType track.RequesterType) bool {
	// Apply to user requests only
	return requesterType == track.RequesterTypeUser
}

// ValidateConfig validates the filter configuration.
func (f *BlacklistArtistFilter) ValidateConfig(config map[string]any) error {
	// Configuration is handled when loading the blacklist file
	return nil
}

// Check checks if the artist is blacklisted.
func (f *BlacklistArtistFilter) Check(
	ctx context.Context,
	req TrackRequest,
	t track.Track,
	l *listener.Session,
) Result {
	for _, artistID := range t.ArtistIDs {
		if _, exists := f.blacklist.ArtistIDs[artistID]; exists {
			return Reject("blacklisted_artist")
		}
	}
	return Accept()
}
