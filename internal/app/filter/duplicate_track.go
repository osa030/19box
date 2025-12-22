package filter

import (
	"context"
	"regexp"
	"strings"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// DuplicateTrackFilter checks for duplicate tracks in the queue.
// Detects:
// - Exact track ID matches
// - Remasters (normalized track name + same artist)
// Excludes:
// - Cover songs (same track name but different artist)
type DuplicateTrackFilter struct {
	queueManager QueueManager
}

// QueueManager interface for accessing queue data.
type QueueManager interface {
	GetAllTracks() []track.QueuedTrack
}

// NewDuplicateTrackFilter creates a new duplicate track filter.
func NewDuplicateTrackFilter(queueManager QueueManager) *DuplicateTrackFilter {
	return &DuplicateTrackFilter{
		queueManager: queueManager,
	}
}

// Name returns the filter name.
func (f *DuplicateTrackFilter) Name() string {
	return "duplicate_track_filter"
}

// Description returns the filter description.
func (f *DuplicateTrackFilter) Description() string {
	return "既にキュー内にある楽曲（リマスター版含む）の重複リクエストを拒否。カバー楽曲は別物として許可"
}

// ReturnCodes returns possible return codes.
func (f *DuplicateTrackFilter) ReturnCodes() []string {
	return []string{"duplicate_track"}
}

// AppliesTo returns which requester types this filter applies to.
func (f *DuplicateTrackFilter) AppliesTo(requesterType track.RequesterType) bool {
	// Apply to user requests only (not to BGM or system tracks)
	return requesterType == track.RequesterTypeUser
}

// ValidateConfig validates the filter configuration.
func (f *DuplicateTrackFilter) ValidateConfig(config map[string]any) error {
	// No configuration needed
	return nil
}

// Check checks if the track is a duplicate.
func (f *DuplicateTrackFilter) Check(
	ctx context.Context,
	req TrackRequest,
	requestedTrack track.Track,
	listenerSession *listener.Session,
) Result {
	queuedTracks := f.queueManager.GetAllTracks()

	for _, queued := range queuedTracks {
		// 1. Exact track ID match
		if queued.Track.ID == requestedTrack.ID {
			return Result{
				Accepted: false,
				Code:     "duplicate_track",
			}
		}

		// 2. Remaster detection: normalized name + same artist
		if f.isRemaster(queued.Track, requestedTrack) {
			return Result{
				Accepted: false,
				Code:     "duplicate_track",
			}
		}
	}

	return Result{Accepted: true}
}

// isRemaster checks if two tracks are the same song (remaster/different version).
// Returns true if:
// - Normalized track names match
// - Main artist is the same
func (f *DuplicateTrackFilter) isRemaster(track1, track2 track.Track) bool {
	// Normalize track names
	name1 := normalizeTrackName(track1.Name)
	name2 := normalizeTrackName(track2.Name)

	// If normalized names don't match, they're different songs
	if name1 != name2 {
		return false
	}

	// Same normalized name - check if same artist
	// If different artists, it's a cover song (allowed)
	return isSameArtist(track1, track2)
}

// normalizeTrackName removes remaster information and version details.
func normalizeTrackName(name string) string {
	// Convert to lowercase
	normalized := strings.ToLower(name)

	// Remove common remaster patterns
	remasterPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\s*-?\s*\d{4}\s+remaster(ed)?`),      // "- 2011 Remaster"
		regexp.MustCompile(`\s*\(remaster(ed)?\s*\d{0,4}\)`),     // "(Remastered 2023)"
		regexp.MustCompile(`\s*\[remaster(ed)?\s*\d{0,4}\]`),     // "[Remastered]"
		regexp.MustCompile(`\s*-?\s*remaster(ed)?(\s+version)?`), // "- Remastered"
		regexp.MustCompile(`\s*\(.*?remaster.*?\)`),              // "(Any Remaster text)"
		regexp.MustCompile(`\s*\[.*?remaster.*?\]`),              // "[Any Remaster text]"
	}

	for _, pattern := range remasterPatterns {
		normalized = pattern.ReplaceAllString(normalized, "")
	}

	// Remove other common version indicators
	versionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\s*\(.*?version\)`),        // "(Single Version)"
		regexp.MustCompile(`\s*\(.*?edit\)`),           // "(Radio Edit)"
		regexp.MustCompile(`\s*-?\s*live`),             // "- Live"
		regexp.MustCompile(`\s*\(live\)`),              // "(Live)"
		regexp.MustCompile(`\s*-?\s*radio\s+edit`),     // "- Radio Edit"
		regexp.MustCompile(`\s*-?\s*single\s+version`), // "- Single Version"
	}

	for _, pattern := range versionPatterns {
		normalized = pattern.ReplaceAllString(normalized, "")
	}

	// Remove extra whitespace
	normalized = strings.TrimSpace(normalized)
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")

	// Remove trailing dashes
	normalized = strings.TrimRight(normalized, " -")

	return normalized
}

// isSameArtist checks if two tracks have the same main artist.
func isSameArtist(track1, track2 track.Track) bool {
	if len(track1.Artists) == 0 || len(track2.Artists) == 0 {
		return false
	}

	// Compare first (main) artist, case-insensitive
	return strings.EqualFold(track1.Artists[0], track2.Artists[0])
}

// Register the filter
func init() {
	// Note: Queue Manager will be injected when filter is instantiated
	// Registration happens in session manager
}
