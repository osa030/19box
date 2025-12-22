// Package listener provides the ListenerSession domain entity.
package listener

import "time"

// Session represents a listener's session in the jukebox.
type Session struct {
	ID             string     // UUID
	DisplayName    string     // Display name
	ExternalUserID string     // External user ID (for bot integration, optional)
	PendingTracks  int        // Number of tracks waiting to be played
	IsKicked       bool       // Kicked status
	JoinedAt       time.Time  // Join time
	VIPStatus      bool       // VIP status (for priority queue)
	RequestQuota   int        // Request quota (N tracks per hour, future extension)
	TotalRequests  int        // Total request count
	LastRequestAt  *time.Time // Last request time
}

// NewSession creates a new listener session.
func NewSession(id, displayName, externalUserID string, isVIP bool) *Session {
	return &Session{
		ID:             id,
		DisplayName:    displayName,
		ExternalUserID: externalUserID,
		PendingTracks:  0,
		IsKicked:       false,
		JoinedAt:       time.Now(),
		VIPStatus:      isVIP,
		RequestQuota:   0,
		TotalRequests:  0,
		LastRequestAt:  nil,
	}
}

// IncrementPendingTracks increments the pending tracks count.
func (s *Session) IncrementPendingTracks() {
	s.PendingTracks++
	s.TotalRequests++
	now := time.Now()
	s.LastRequestAt = &now
}

// DecrementPendingTracks decrements the pending tracks count.
// Called when a track starts playing.
func (s *Session) DecrementPendingTracks() {
	if s.PendingTracks > 0 {
		s.PendingTracks--
	}
}

// Kick marks the listener as kicked.
func (s *Session) Kick() {
	s.IsKicked = true
}

// CanRequest checks if the listener can make a request.
// Returns false if kicked or has pending tracks (non-VIP only).
func (s *Session) CanRequest() bool {
	if s.IsKicked {
		return false
	}
	if s.VIPStatus {
		return true
	}
	return s.PendingTracks == 0
}
