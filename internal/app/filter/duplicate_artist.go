package filter

import (
	"context"
	"strings"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// DuplicateArtistFilter checks if any artist of the requested track is already in the queue.
// Unlike DuplicateTrackFilter which checks for the same track, this filter prevents
// multiple tracks from the same artist in the queue.
type DuplicateArtistFilter struct {
	queueManager QueueManager
}

// NewDuplicateArtistFilter creates a new duplicate artist filter.
func NewDuplicateArtistFilter(queueManager QueueManager) *DuplicateArtistFilter {
	return &DuplicateArtistFilter{
		queueManager: queueManager,
	}
}

// Name returns the filter name.
func (f *DuplicateArtistFilter) Name() string {
	return "duplicate_artist_filter"
}

// Description returns the filter description.
func (f *DuplicateArtistFilter) Description() string {
	return "Rejects track requests if the artist already has a track in the queue"
}

// ReturnCodes returns possible return codes.
func (f *DuplicateArtistFilter) ReturnCodes() []string {
	return []string{"duplicate_artist"}
}

// AppliesTo returns which requester types this filter applies to.
func (f *DuplicateArtistFilter) AppliesTo(requesterType track.RequesterType) bool {
	// Apply to user requests only (not to BGM or system tracks)
	return requesterType == track.RequesterTypeUser
}

// ValidateConfig validates the filter configuration.
func (f *DuplicateArtistFilter) ValidateConfig(config map[string]any) error {
	// No configuration needed
	return nil
}

// Check checks if any artist of the requested track is already in the queue.
func (f *DuplicateArtistFilter) Check(
	ctx context.Context,
	req TrackRequest,
	requestedTrack track.Track,
	listenerSession *listener.Session,
) Result {
	// Build a set of artists currently in the queue (case-insensitive)
	queuedArtists := make(map[string]struct{})
	for _, queued := range f.queueManager.GetAllTracks() {
		for _, artist := range queued.Track.Artists {
			queuedArtists[strings.ToLower(artist)] = struct{}{}
		}
	}

	// Check if any artist of the requested track is in the queue
	for _, artist := range requestedTrack.Artists {
		if _, exists := queuedArtists[strings.ToLower(artist)]; exists {
			return Reject("duplicate_artist")
		}
	}

	return Accept()
}

// Register the filter
func init() {
	// Note: Queue Manager will be injected when filter is instantiated
	// Registration happens in session manager
}
