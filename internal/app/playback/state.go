// Package playback provides playback control with integrated queue management.
package playback

// State represents the playback state.
type State int

const (
	StateIdle    State = iota // No track playing (queue empty or stopped)
	StatePlaying              // Track is playing
	StatePaused               // Track is paused
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StatePlaying:
		return "playing"
	case StatePaused:
		return "paused"
	default:
		return "unknown"
	}
}
