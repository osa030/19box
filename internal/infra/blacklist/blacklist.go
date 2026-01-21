package blacklist

import (
	"os"

	"github.com/cockroachdb/errors"
	"gopkg.in/yaml.v3"
)

// Blacklist holds blacklisted items.
type Blacklist struct {
	TrackIDs  map[string]struct{}
	ArtistIDs map[string]struct{}
}

// Config represents the blacklist configuration file structure.
type Config struct {
	Tracks  []string `yaml:"tracks"`
	Artists []string `yaml:"artists"`
}

// Load loads the blacklist from a YAML file.
func Load(path string) (*Blacklist, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read blacklist file")
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrap(err, "failed to parse blacklist file")
	}

	// Initialize maps
	trackIDs := make(map[string]struct{}, len(cfg.Tracks))
	for _, id := range cfg.Tracks {
		trackIDs[id] = struct{}{}
	}

	artistIDs := make(map[string]struct{}, len(cfg.Artists))
	for _, id := range cfg.Artists {
		artistIDs[id] = struct{}{}
	}

	return &Blacklist{
		TrackIDs:  trackIDs,
		ArtistIDs: artistIDs,
	}, nil
}
