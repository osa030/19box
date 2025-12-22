package filter

import (
	"context"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// MarketFilter checks if the track is available in the configured market.
type MarketFilter struct {
	market string
}

// NewMarketFilter creates a new MarketFilter with the specified market.
func NewMarketFilter(market string) *MarketFilter {
	return &MarketFilter{market: market}
}

func (f *MarketFilter) Name() string {
	return "market_filter"
}

func (f *MarketFilter) Description() string {
	return "Checks if the track is available in the configured market"
}

func (f *MarketFilter) ReturnCodes() []string {
	return []string{"market_restriction"}
}

func (f *MarketFilter) ValidateConfig(settings map[string]any) error {
	return nil
}

func (f *MarketFilter) AppliesTo(requesterType track.RequesterType) bool {
	// Market restrictions apply to all tracks regardless of source
	return true
}

func (f *MarketFilter) Check(ctx context.Context, req TrackRequest, t track.Track, l *listener.Session) Result {
	if f.market == "" {
		return Accept()
	}

	if !t.IsAvailableInMarket(f.market) {
		return Reject("market_restriction")
	}
	return Accept()
}

func init() {
	// Note: This filter requires market config, so we register a placeholder.
	// The actual filter is created with config in session manager.
}
