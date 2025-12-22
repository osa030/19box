package registry

import (
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/osa030/19box/internal/domain/listener"
)

var (
	ErrInvalidListener = errors.New("invalid listener")
	ErrListenerKicked  = errors.New("listener is kicked")
)

// ListenerRegistry manages listener sessions with thread-safe access.
type ListenerRegistry struct {
	mu        sync.RWMutex
	listeners map[string]*listener.Session
}

// NewListenerRegistry creates a new listener registry.
func NewListenerRegistry() *ListenerRegistry {
	return &ListenerRegistry{
		listeners: make(map[string]*listener.Session),
	}
}

// Join adds a new listener and returns their session ID.
func (r *ListenerRegistry) Join(displayName, externalUserID string, isVIP bool) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already joined (by external ID)
	if externalUserID != "" {
		for _, session := range r.listeners {
			if session.ExternalUserID == externalUserID && !session.IsKicked {
				return session.ID, nil
			}
		}
	}

	// Generate new ID
	id := uuid.New().String()
	session := listener.NewSession(id, displayName, externalUserID, isVIP)
	r.listeners[id] = session

	return id, nil
}

// Get retrieves a listener session by ID.
func (r *ListenerRegistry) Get(listenerID string) (*listener.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.listeners[listenerID]
	if !ok {
		return nil, ErrInvalidListener
	}
	return session, nil
}

// Validate checks if a listener exists and is valid (not kicked).
func (r *ListenerRegistry) Validate(listenerID string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.listeners[listenerID]
	if !ok {
		return ErrInvalidListener
	}
	if session.IsKicked {
		return ErrListenerKicked
	}
	return nil
}

// Kick marks a listener as kicked.
func (r *ListenerRegistry) Kick(listenerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.listeners[listenerID]
	if !ok {
		return ErrInvalidListener
	}
	session.Kick()
	return nil
}

// IncrementPending increments a listener's pending track count.
func (r *ListenerRegistry) IncrementPending(listenerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.listeners[listenerID]
	if !ok {
		return ErrInvalidListener
	}
	session.IncrementPendingTracks()
	return nil
}

// DecrementPending decrements a listener's pending track count.
func (r *ListenerRegistry) DecrementPending(listenerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session, ok := r.listeners[listenerID]; ok {
		session.DecrementPendingTracks()
	}
}

// All returns all listener sessions.
func (r *ListenerRegistry) All() []*listener.Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*listener.Session, 0, len(r.listeners))
	for _, session := range r.listeners {
		result = append(result, session)
	}
	return result
}

// Count returns the number of listeners.
func (r *ListenerRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.listeners)
}
