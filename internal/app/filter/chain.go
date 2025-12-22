// Package filter provides the filter chain for request validation.
package filter

import (
	"context"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// Chain executes filters in sequence.
type Chain struct {
	filters []Filter
}

// NewChain creates a new filter chain.
func NewChain() *Chain {
	return &Chain{
		filters: make([]Filter, 0),
	}
}

// Add adds a filter to the chain.
func (c *Chain) Add(f Filter) {
	c.filters = append(c.filters, f)
}

// Execute runs all filters in sequence.
// Returns immediately if any filter rejects the request.
// Filters are only applied if they declare they apply to the given requester type.
func (c *Chain) Execute(ctx context.Context, req TrackRequest, t track.Track, l *listener.Session, requesterType track.RequesterType) Result {
	for _, f := range c.filters {
		// Skip filters that don't apply to this requester type
		if !f.AppliesTo(requesterType) {
			continue
		}

		result := f.Check(ctx, req, t, l)
		if !result.Accepted {
			return result
		}
	}
	return Accept()
}

// Filters returns all filters in the chain.
func (c *Chain) Filters() []Filter {
	return c.filters
}
