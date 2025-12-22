package bgm

import (
	"context"

	"github.com/cockroachdb/errors"
	zlog "github.com/rs/zerolog/log"

	"github.com/osa030/19box/internal/domain/track"
)

// CandidateWithSource represents a track candidate with its source provider info.
type CandidateWithSource struct {
	Track       track.Track
	DisplayName string
}

// ProviderWithMetadata wraps a provider with its metadata.
type ProviderWithMetadata struct {
	Provider    Provider
	DisplayName string
}

// ProviderChain tries multiple providers in order until enough candidates are found.
type ProviderChain struct {
	providers []ProviderWithMetadata
}

// NewProviderChain creates a new provider chain.
func NewProviderChain(providers []ProviderWithMetadata) *ProviderChain {
	return &ProviderChain{
		providers: providers,
	}
}

// GetCandidates retrieves candidates from all providers.
// All providers are tried to maximize candidate pool for filtering.
func (c *ProviderChain) GetCandidates(ctx context.Context, count int, seedTracks []track.Track, excludeIDs map[string]bool) ([]CandidateWithSource, error) {
	var allCandidates []CandidateWithSource
	currentExcludeIDs := make(map[string]bool)
	for k, v := range excludeIDs {
		currentExcludeIDs[k] = v
	}

	for i, pm := range c.providers {
		zlog.Debug().Msgf("trying provider: index=%d total=%d name=%s provider_type=%s",
			i+1, len(c.providers), pm.DisplayName, pm.Provider.Name())

		candidates, err := pm.Provider.GetCandidates(ctx, count, seedTracks, currentExcludeIDs)
		if err != nil {
			zlog.Warn().Msgf("provider failed, trying next: provider=%s error=%v", pm.DisplayName, err)
			continue
		}

		if len(candidates) == 0 {
			zlog.Debug().Msgf("provider returned no candidates: provider=%s", pm.DisplayName)
			continue
		}

		// Add candidates with source info
		for _, t := range candidates {
			allCandidates = append(allCandidates, CandidateWithSource{
				Track:       t,
				DisplayName: pm.DisplayName,
			})
			// Update exclude set to avoid duplicates from next provider
			currentExcludeIDs[t.ID] = true
		}

		zlog.Info().Msgf("provider returned candidates: provider=%s count=%d total_so_far=%d",
			pm.DisplayName, len(candidates), len(allCandidates))
	}

	if len(allCandidates) == 0 {
		return nil, errors.New("all providers failed to return candidates")
	}

	return allCandidates, nil
}

// Name returns the chain name.
func (c *ProviderChain) Name() string {
	return "provider_chain"
}
