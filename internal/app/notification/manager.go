// Package notification provides the notification manager for broadcasting events.
package notification

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/puzpuzpuz/xsync/v3"

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
	subscriptions *xsync.MapOf[string, *subscription]
	sequenceNo    uint64
}

// NewManager creates a new notification manager.
func NewManager() *Manager {
	return &Manager{
		subscriptions: xsync.NewMapOf[string, *subscription](),
	}
}

// Subscribe adds a new subscription and returns the subscription ID and current sequence number.
func (m *Manager) Subscribe(stream Stream) (string, uint64) {
	seq := atomic.LoadUint64(&m.sequenceNo)

	id := uuid.New().String()
	m.subscriptions.Store(id, &subscription{
		id:     id,
		stream: stream,
	})
	return id, seq
}

// Unsubscribe removes a subscription.
func (m *Manager) Unsubscribe(subscriptionID string) {
	m.subscriptions.Delete(subscriptionID)
}

// Broadcast sends a notification to all subscribers.
// Each stream send is done in a goroutine with a timeout to prevent blocking.
func (m *Manager) Broadcast(notification *jukeboxv1.Notification) error {
	// シーケンス番号を取得してインクリメント
	currentSequenceNo := atomic.AddUint64(&m.sequenceNo, 1)

	// 通知にシーケンス番号を付与
	notification.SequenceNo = currentSequenceNo

	// Send to each subscriber in parallel with timeout
	var wg sync.WaitGroup
	m.subscriptions.Range(func(key string, s *subscription) bool {
		wg.Add(1)
		go func(sub *subscription) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- sub.stream.Send(notification)
			}()

			select {
			case err := <-done:
				if err != nil {
					// Error is silently ignored - subscription will be cleaned up on next failure
				}
			case <-ctx.Done():
				// Timeout - continue to next subscriber
			}
		}(s)
		return true
	})

	// Wait for all sends to complete or timeout
	wg.Wait()
	return nil
}

// Send sends a notification to a specific subscriber.
func (m *Manager) Send(subscriptionID string, notification *jukeboxv1.Notification) error {
	if sub, ok := m.subscriptions.Load(subscriptionID); ok {
		return sub.stream.Send(notification)
	}
	return nil
}

// SubscriberCount returns the number of active subscribers.
func (m *Manager) SubscriberCount() int {
	return m.subscriptions.Size()
}

// Close closes the manager and removes all subscriptions.
func (m *Manager) Close() {
	m.subscriptions = xsync.NewMapOf[string, *subscription]()
}
