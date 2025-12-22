// Package filter provides the filter chain for request validation.
package filter

import (
	"context"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// TrackRequest represents a track request to be validated.
type TrackRequest struct {
	ListenerID string
	TrackID    string
}

// Result represents the result of a filter check.
type Result struct {
	Accepted bool
	Code     string // e.g., "user_pending", "kicked", "market_restriction"
}

// Accept returns an accepted result.
func Accept() Result {
	return Result{Accepted: true}
}

// Reject returns a rejected result with the given code.
func Reject(code string) Result {
	return Result{Accepted: false, Code: code}
}

// Filter is the interface for request filters.
type Filter interface {
	// Name returns the filter name (used in config).
	Name() string
	// Description returns a human-readable description.
	Description() string
	// ReturnCodes returns the codes this filter can return.
	ReturnCodes() []string
	// ValidateConfig validates the filter configuration.
	ValidateConfig(settings map[string]any) error
	// AppliesTo returns true if this filter should be applied to the given requester type.
	AppliesTo(requesterType track.RequesterType) bool
	// Check performs the filter check.
	Check(ctx context.Context, req TrackRequest, t track.Track, l *listener.Session) Result
}

// registry holds registered filter factories.
var registry = make(map[string]func() Filter)

// Register registers a filter factory.
func Register(name string, factory func() Filter) {
	registry[name] = factory
}

// GetRegistered returns all registered filter factories.
func GetRegistered() map[string]func() Filter {
	return registry
}
