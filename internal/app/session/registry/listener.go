package registry

import (
	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/osa030/19box/internal/domain/listener"
	"github.com/puzpuzpuz/xsync/v3"
)

var (
	ErrInvalidListener = errors.New("invalid listener")
	ErrListenerKicked  = errors.New("listener is kicked")
)

// ListenerRegistry manages listener sessions with thread-safe access.
type ListenerRegistry struct {
	mu        *xsync.RBMutex
	listeners map[string]*listener.Session
}

// NewListenerRegistry creates a new listener registry.
func NewListenerRegistry() *ListenerRegistry {
	return &ListenerRegistry{
		mu:        xsync.NewRBMutex(),
		listeners: make(map[string]*listener.Session),
	}
}

// Join adds a new listener and returns their session ID.
func (r *ListenerRegistry) Join(displayName, externalUserID string, isVIP bool) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for existing session
	// 1. By external user ID (bot users, etc.)
	if externalUserID != "" {
		for _, session := range r.listeners {
			if session.ExternalUserID == externalUserID {
				if session.IsKicked {
					return "", ErrListenerKicked
				}
				return session.ID, nil
			}
		}
	} else {
		// 2. By display name (web users without external ID)
		for _, session := range r.listeners {
			if session.DisplayName == displayName && session.ExternalUserID == "" {
				if session.IsKicked {
					return "", ErrListenerKicked
				}
				return session.ID, nil
			}
		}
	}

	// Create new session if not found
	id := uuid.New().String()
	session := listener.NewSession(id, displayName, externalUserID, isVIP)
	r.listeners[id] = session

	return id, nil
}

// Get retrieves a listener session by ID.
func (r *ListenerRegistry) Get(listenerID string) (*listener.Session, error) {
	token := r.mu.RLock()
	defer r.mu.RUnlock(token)

	session, ok := r.listeners[listenerID]
	if !ok {
		return nil, ErrInvalidListener
	}
	return session, nil
}

// Validate checks if a listener exists and is valid (not kicked).
func (r *ListenerRegistry) Validate(listenerID string) error {
	token := r.mu.RLock()
	defer r.mu.RUnlock(token)

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
	token := r.mu.RLock()
	defer r.mu.RUnlock(token)

	result := make([]*listener.Session, 0, len(r.listeners))
	for _, session := range r.listeners {
		result = append(result, session)
	}
	return result
}

// Count returns the number of listeners.
func (r *ListenerRegistry) Count() int {
	token := r.mu.RLock()
	defer r.mu.RUnlock(token)
	return len(r.listeners)
}
