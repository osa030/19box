package state

import (
	"sync"
	"time"

	jukeboxv1 "github.com/osa030/19box/internal/gen/jukebox/v1"
)

// Manager manages session state with thread-safe access.
type Manager struct {
	mu sync.RWMutex

	// Session identity
	sessionID    string
	playlistID   string
	playlistURL  string
	playlistName string

	// Session lifecycle
	phase     Phase
	accepting AcceptingState

	// Schedule
	startTime      *time.Time
	endTime        *time.Time
	endingDuration time.Duration

	// Metadata
	keywords []string
}

// New creates a new state manager.
func New(sessionID string) *Manager {
	return &Manager{
		sessionID: sessionID,
		phase:     PhaseWaiting,
		accepting: NotAccepting,
	}
}

// GetPhase returns the current session phase.
func (m *Manager) GetPhase() Phase {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.phase
}

// SetPhase sets the session phase.
func (m *Manager) SetPhase(p Phase) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.phase = p
}

// IsAccepting returns true if the session is accepting requests.
func (m *Manager) IsAccepting() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accepting == Accepting
}

// GetAcceptingState returns the accepting state.
func (m *Manager) GetAcceptingState() AcceptingState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accepting
}

// SetAccepting sets the accepting state.
func (m *Manager) SetAccepting(a AcceptingState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accepting = a
}

// StartAccepting sets the accepting state to Accepting.
func (m *Manager) StartAccepting() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accepting = Accepting
}

// StopAccepting sets the accepting state to NotAccepting.
func (m *Manager) StopAccepting() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accepting = NotAccepting
}

// CanAcceptRequests returns true if the session can accept requests.
// This is true when the session is active and accepting.
func (m *Manager) CanAcceptRequests() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.phase == PhaseActive && m.accepting == Accepting
}

// GetSessionID returns the session ID.
func (m *Manager) GetSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionID
}

// GetPlaylistURL returns the playlist URL.
func (m *Manager) GetPlaylistURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.playlistURL
}

// GetPlaylistID returns the playlist ID.
func (m *Manager) GetPlaylistID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.playlistID
}

// SetPlaylistInfo sets playlist information.
func (m *Manager) SetPlaylistInfo(id, url, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.playlistID = id
	m.playlistURL = url
	m.playlistName = name
}

// SetKeywords sets the session keywords.
func (m *Manager) SetKeywords(keywords []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keywords = keywords
}

// SetTimes sets the start and end times.
func (m *Manager) SetTimes(start, end *time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startTime = start
	m.endTime = end
}

// GetTimes returns the start and end times.
func (m *Manager) GetTimes() (*time.Time, *time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startTime, m.endTime
}

// SetEndingDuration sets the ending duration.
func (m *Manager) SetEndingDuration(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.endingDuration = d
}

// GetEndingDuration returns the ending duration.
func (m *Manager) GetEndingDuration() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.endingDuration
}

// BuildSessionInfo creates a complete SessionInfo with all fields.
func (m *Manager) BuildSessionInfo() *jukeboxv1.SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.buildSessionInfoLocked()
}

// buildSessionInfoLocked creates SessionInfo without acquiring lock.
// Must be called with m.mu held (either RLock or Lock).
func (m *Manager) buildSessionInfoLocked() *jukeboxv1.SessionInfo {
	var startTimeStr string
	if m.startTime != nil {
		startTimeStr = m.startTime.Format(time.RFC3339)
	}

	var endTimeStr string
	if m.endTime != nil {
		endTimeStr = m.endTime.Format(time.RFC3339)
	}

	return &jukeboxv1.SessionInfo{
		SessionId:          m.sessionID,
		PlaylistName:       m.playlistName,
		PlaylistUrl:        m.playlistURL,
		Keywords:           m.keywords,
		ScheduledStartTime: startTimeStr,
		ScheduledEndTime:   endTimeStr,
		AcceptingRequests:  m.accepting == Accepting,
	}
}
