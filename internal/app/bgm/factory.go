package bgm

import (
	"github.com/cockroachdb/errors"
	zlog "github.com/rs/zerolog/log"

	"github.com/osa030/19box/internal/infra/config"
)

// NewProviderChainFromConfig creates a provider chain from configuration.
func NewProviderChainFromConfig(cfg *config.Config, spotify SpotifyClient) (*ProviderChain, error) {
	if len(cfg.BGM.Providers) == 0 {
		return nil, errors.New("no BGM providers configured")
	}

	var providers []ProviderWithMetadata

	for i, pcfg := range cfg.BGM.Providers {
		var provider Provider
		var err error
		zlog.Debug().Msgf("creating BGM provider: index=%d type=%s settings=%+v", i+1, pcfg.Type, pcfg.Settings)
		switch pcfg.Type {
		case "playlist":
			provider, err = NewPlaylistProvider(spotify, cfg.BGM.CandidateCount, pcfg.Settings)

		case "lastfm":
			provider, err = NewLastFmProvider(spotify, cfg.BGM.CandidateCount, pcfg.Settings)

		default:
			return nil, errors.Newf("unsupported provider type: %s (provider index %d)", pcfg.Type, i)
		}

		if err != nil {
			return nil, errors.Wrapf(err, "failed to create provider (index %d, type %s)", i, pcfg.Type)
		}

		providers = append(providers, ProviderWithMetadata{
			Provider:    provider,
			DisplayName: pcfg.DisplayName,
		})

		zlog.Info().Msgf("registered BGM provider: index=%d type=%s display_name=%s", i+1, pcfg.Type, pcfg.DisplayName)
	}

	return NewProviderChain(providers), nil
}
