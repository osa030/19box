// Package playlist provides the Playlist domain entity.
package playlist

import "github.com/osa030/19box/internal/domain/track"

// Playlist represents a Spotify playlist.
type Playlist struct {
	ID          string        // Spotify Playlist ID
	Name        string        // Playlist name
	Description string        // Playlist description
	URL         string        // Spotify URL
	Tracks      []track.Track // Tracks in the playlist
}

// TrackIDs returns all track IDs in the playlist.
func (p *Playlist) TrackIDs() []string {
	ids := make([]string, len(p.Tracks))
	for i, t := range p.Tracks {
		ids[i] = t.ID
	}
	return ids
}

// TotalDuration returns the total duration of all tracks.
func (p *Playlist) TotalDuration() int64 {
	var total int64
	for _, t := range p.Tracks {
		total += int64(t.Duration.Seconds())
	}
	return total
}
