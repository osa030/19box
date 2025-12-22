package playback

import "github.com/osa030/19box/internal/domain/track"

// EventType represents a playback event type.
type EventType int

const (
	EventTrackStarted   EventType = iota // Track started playing
	EventTrackEnded                      // Track finished playing
	EventTrackSkipped                    // Track was skipped
	EventStateChanged                    // Playback state changed (pause/resume)
	EventQueueDepleting                  // Queue is depleting (remaining time below threshold)
	EventQueueEmpty                      // Queue became empty
)

// String returns the string representation of the event type.
func (e EventType) String() string {
	switch e {
	case EventTrackStarted:
		return "track_started"
	case EventTrackEnded:
		return "track_ended"
	case EventTrackSkipped:
		return "track_skipped"
	case EventStateChanged:
		return "state_changed"
	case EventQueueDepleting:
		return "queue_depleting"
	case EventQueueEmpty:
		return "queue_empty"
	default:
		return "unknown"
	}
}

// Event represents a playback event.
type Event struct {
	Type  EventType
	Track *track.QueuedTrack // Current track (nil for some events)
	State State              // Current playback state
}
