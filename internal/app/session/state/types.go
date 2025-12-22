// Package state provides session state management.
package state

// Phase represents the session lifecycle phase.
type Phase int

const (
	PhaseWaiting    Phase = iota // Waiting for start time
	PhaseActive                  // Session is active
	PhaseEnding                  // Playing ending tracks
	PhaseTerminated              // Session has ended
)

// String returns the string representation of the phase.
func (p Phase) String() string {
	switch p {
	case PhaseWaiting:
		return "waiting"
	case PhaseActive:
		return "active"
	case PhaseEnding:
		return "ending"
	case PhaseTerminated:
		return "terminated"
	default:
		return "unknown"
	}
}

// AcceptingState represents whether requests are being accepted.
type AcceptingState int

const (
	NotAccepting AcceptingState = iota // Not accepting requests
	Accepting                          // Accepting requests
)

// String returns the string representation of the accepting state.
func (a AcceptingState) String() string {
	switch a {
	case NotAccepting:
		return "not_accepting"
	case Accepting:
		return "accepting"
	default:
		return "unknown"
	}
}
