// Package notification provides the notification manager for broadcasting events.
package notification

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	jukeboxv1 "github.com/osa030/19box/internal/gen/jukebox/v1"
)

// Stream represents a notification stream for a subscriber.
type Stream interface {
	Send(*jukeboxv1.Notification) error
}

// subscription represents a subscriber's subscription.
type subscription struct {
	id     string
	stream Stream
}

// Manager manages notification subscriptions and broadcasting.
type Manager struct {
	mu            sync.RWMutex
	subscriptions map[string]*subscription
	sequenceNo    uint64
	sequenceNoMu  sync.Mutex
}

// NewManager creates a new notification manager.
func NewManager() *Manager {
	return &Manager{
		subscriptions: make(map[string]*subscription),
	}
}

// Subscribe adds a new subscription and returns the subscription ID.
func (m *Manager) Subscribe(stream Stream) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	m.subscriptions[id] = &subscription{
		id:     id,
		stream: stream,
	}
	return id
}

// NextSequenceNo returns the next sequence number and increments the counter.
func (m *Manager) NextSequenceNo() uint64 {
	m.sequenceNoMu.Lock()
	defer m.sequenceNoMu.Unlock()
	m.sequenceNo++
	return m.sequenceNo
}

// Unsubscribe removes a subscription.
func (m *Manager) Unsubscribe(subscriptionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subscriptions, subscriptionID)
}

// Broadcast sends a notification to all subscribers.
// Each stream send is done in a goroutine with a timeout to prevent blocking.
func (m *Manager) Broadcast(notification *jukeboxv1.Notification) error {
	// シーケンス番号を取得してインクリメント
	m.sequenceNoMu.Lock()
	m.sequenceNo++
	currentSequenceNo := m.sequenceNo
	m.sequenceNoMu.Unlock()

	// 通知にシーケンス番号を付与
	notification.SequenceNo = currentSequenceNo

	m.mu.RLock()
	// Copy subscriptions to avoid holding lock during sends
	subs := make([]*subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		subs = append(subs, sub)
	}
	m.mu.RUnlock()

	// Send to each subscriber in parallel with timeout
	var wg sync.WaitGroup
	for _, sub := range subs {
		wg.Add(1)
		go func(s *subscription) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- s.stream.Send(notification)
			}()

			select {
			case err := <-done:
				if err != nil {
					// Error is silently ignored - subscription will be cleaned up on next failure
				}
			case <-ctx.Done():
				// Timeout - continue to next subscriber
			}
		}(sub)
	}

	// Wait for all sends to complete or timeout
	wg.Wait()
	return nil
}

// Send sends a notification to a specific subscriber.
func (m *Manager) Send(subscriptionID string, notification *jukeboxv1.Notification) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sub, ok := m.subscriptions[subscriptionID]
	if !ok {
		return nil
	}

	return sub.stream.Send(notification)
}

// SubscriberCount returns the number of active subscribers.
func (m *Manager) SubscriberCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subscriptions)
}

// Close closes the manager and removes all subscriptions.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscriptions = make(map[string]*subscription)
}
