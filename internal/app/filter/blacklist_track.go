package filter

import (
	"context"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
	"github.com/osa030/19box/internal/infra/blacklist"
)

// BlacklistTrackFilter checks if the track is blacklisted.
type BlacklistTrackFilter struct {
	blacklist *blacklist.Blacklist
}

// NewBlacklistTrackFilter creates a new blacklist track filter.
func NewBlacklistTrackFilter(blacklist *blacklist.Blacklist) *BlacklistTrackFilter {
	return &BlacklistTrackFilter{
		blacklist: blacklist,
	}
}

// Name returns the filter name.
func (f *BlacklistTrackFilter) Name() string {
	return "blacklist_track_filter"
}

// Description returns the filter description.
func (f *BlacklistTrackFilter) Description() string {
	return "Rejects track requests if the track ID is in the blacklist"
}

// ReturnCodes returns possible return codes.
func (f *BlacklistTrackFilter) ReturnCodes() []string {
	return []string{"blacklisted_track"}
}

// AppliesTo returns which requester types this filter applies to.
func (f *BlacklistTrackFilter) AppliesTo(requesterType track.RequesterType) bool {
	// Apply to user requests only
	return requesterType == track.RequesterTypeUser
}

// ValidateConfig validates the filter configuration.
func (f *BlacklistTrackFilter) ValidateConfig(config map[string]any) error {
	// Configuration is handled when loading the blacklist file
	return nil
}

// Check checks if the track is blacklisted.
func (f *BlacklistTrackFilter) Check(
	ctx context.Context,
	req TrackRequest,
	t track.Track,
	l *listener.Session,
) Result {
	if _, exists := f.blacklist.TrackIDs[t.ID]; exists {
		return Reject("blacklisted_track")
	}
	return Accept()
}
