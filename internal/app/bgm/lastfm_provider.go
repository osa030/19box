package bgm

import (
	"context"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"

	"github.com/osa030/19box/internal/domain/track"
	"github.com/osa030/19box/internal/infra/lastfm"
)

// LastFmClient defines the interface for Last.fm operations.
type LastFmClient interface {
	GetSimilarTracks(ctx context.Context, trackName, artistName string, limit int) ([]lastfm.SimilarTrack, error)
	GetTopTags(ctx context.Context, trackName, artistName string, limit int) ([]lastfm.Tag, error)
	GetTopTracks(ctx context.Context, tagName string, limit int) ([]lastfm.TopTrack, error)
	GetChartTopTracks(ctx context.Context, limit int) ([]lastfm.TopTrack, error)
}

type LastFmProviderConfig struct {
	APIKey         string  `yaml:"api_key" mapstructure:"api_key" validate:"required"`
	SeedTrackCount int     `yaml:"seed_track_count" mapstructure:"seed_track_count" default:"3" validate:"gte=1"`
	TagCount       int     `yaml:"tag_count" mapstructure:"tag_count" default:"5" validate:"gte=1"`
	TagWeight      float64 `yaml:"tag_weight" mapstructure:"tag_weight" default:"0.4" validate:"lte=1.0"`
	SimilarWeight  float64 `yaml:"similar_weight" mapstructure:"similar_weight" default:"0.6" validate:"lte=1.0"`
}

// LastFmProvider provides BGM tracks using Last.fm API with hybrid scoring.
// Combines tag-based and similar-based strategies with configurable weights.
type LastFmProvider struct {
	lastfm  LastFmClient
	spotify SpotifyClient

	// Cache for Spotify search results
	spotifySearchCache map[string]*track.Track
	candidateCache     []track.Track
	cacheMutex         sync.RWMutex

	// Configuration
	candidateCount int
	config         *LastFmProviderConfig
}

// ScoredTrack represents a track with its hybrid score.
type ScoredTrack struct {
	Track track.Track
	Score float64
}

// NewLastFmProvider creates a new LastFmProvider.
func NewLastFmProvider(spotify SpotifyClient, candidateCount int, settings map[string]any) (*LastFmProvider, error) {
	if spotify == nil {
		return nil, errors.New("spotify client is required")
	}
	if len(settings) == 0 {
		return nil, errors.New("settings are required")
	}

	var config LastFmProviderConfig
	if err := mapstructure.Decode(settings, &config); err != nil {
		return nil, errors.Wrap(err, "failed to decode settings")
	}
	if err := defaults.Set(&config); err != nil {
		return nil, errors.Wrap(err, "failed to set defaults")
	}
	if err := validator.New().Struct(config); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}
	if config.TagWeight+config.SimilarWeight != 1.0 {
		return nil, errors.New("tag weight and similar weight must sum to 1.0")
	}

	lastfmClient, err := lastfm.New(lastfm.Config{APIKey: config.APIKey})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create last.fm client")
	}

	return &LastFmProvider{
		spotify:            spotify,
		lastfm:             lastfmClient,
		spotifySearchCache: make(map[string]*track.Track),
		candidateCache:     make([]track.Track, 0),
		cacheMutex:         sync.RWMutex{},
		candidateCount:     candidateCount,

		config: &config}, nil
}

// GetCandidates retrieves BGM track candidates using hybrid scoring.
func (p *LastFmProvider) GetCandidates(ctx context.Context, count int, seedTracks []track.Track, existingTrackIDs map[string]bool) ([]track.Track, error) {
	if count <= 0 {
		return []track.Track{}, nil
	}

	// Limit seed tracks
	if len(seedTracks) > p.config.SeedTrackCount {
		seedTracks = seedTracks[:p.config.SeedTrackCount]
	}

	if len(seedTracks) == 0 {
		// No seed tracks available, use global charts as fallback
		return p.getChartBasedCandidates(ctx, count, existingTrackIDs)
	}

	// 1. Get tag-based candidates
	tagCandidates := p.getTagBasedCandidates(ctx, seedTracks, existingTrackIDs)

	// 2. Get similar-based candidates
	similarCandidates := p.getSimilarBasedCandidates(ctx, seedTracks, existingTrackIDs)

	// 3. Score and merge
	scored := p.scoreAndMerge(tagCandidates, similarCandidates)

	if len(scored) == 0 {
		return []track.Track{}, nil
	}

	// 4. Sort by score (descending)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// 5. Return random selection from top N*2 candidates to add variety
	// Instead of always taking the absolute top N, we take top N*2 and pick randomly
	poolSize := count * 2
	if poolSize > len(scored) {
		poolSize = len(scored)
	}

	topCandidates := scored[:poolSize]

	// Initialize RNG
	var cryptoSeed int64
	var buf [8]byte
	if _, err := cryptoRand.Read(buf[:]); err == nil {
		cryptoSeed = int64(binary.LittleEndian.Uint64(buf[:]))
	} else {
		cryptoSeed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(cryptoSeed))

	// Shuffle top candidates
	rng.Shuffle(len(topCandidates), func(i, j int) {
		topCandidates[i], topCandidates[j] = topCandidates[j], topCandidates[i]
	})

	result := make([]track.Track, 0, count)
	for i := 0; i < count && i < len(topCandidates); i++ {
		result = append(result, topCandidates[i].Track)
	}

	return result, nil
}

// Name returns the provider name.
func (p *LastFmProvider) Name() string {
	return "lastfm"
}

// getTagBasedCandidates retrieves candidates using tag-based strategy.
func (p *LastFmProvider) getTagBasedCandidates(ctx context.Context, seedTracks []track.Track, existingTrackIDs map[string]bool) []track.Track {
	// Collect tags from seed tracks
	tagCounts := make(map[string]int)
	for _, seed := range seedTracks {
		if len(seed.Artists) == 0 {
			continue
		}
		tags, err := p.lastfm.GetTopTags(ctx, seed.Name, seed.Artists[0], 10)
		if err != nil {
			continue // Skip on error
		}
		for _, tag := range tags {
			tagCounts[tag.Name] += tag.Count
		}
	}

	if len(tagCounts) == 0 {
		return []track.Track{}
	}

	// Sort tags by count and take top N
	topTags := p.sortAndTakeTopTags(tagCounts, p.config.TagCount)

	// Get tracks for each tag
	var candidates []track.Track
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, tagName := range topTags {
		wg.Add(1)
		go func(tag string) {
			defer wg.Done()
			lfmTracks, err := p.lastfm.GetTopTracks(ctx, tag, 20)
			if err != nil {
				return // Skip on error
			}

			for _, lfmTrack := range lfmTracks {
				spotifyTrack := p.searchOnSpotify(ctx, lfmTrack.Name, lfmTrack.Artist)
				if spotifyTrack != nil {
					mu.Lock()
					if !existingTrackIDs[spotifyTrack.ID] {
						candidates = append(candidates, *spotifyTrack)
					}
					mu.Unlock()
				}
			}
		}(tagName)
	}
	wg.Wait()

	return p.deduplicateByID(candidates)
}

// getSimilarBasedCandidates retrieves candidates using similar-based strategy.
func (p *LastFmProvider) getSimilarBasedCandidates(ctx context.Context, seedTracks []track.Track, existingTrackIDs map[string]bool) []track.Track {
	var candidates []track.Track
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, seed := range seedTracks {
		if len(seed.Artists) == 0 {
			continue
		}

		wg.Add(1)
		go func(s track.Track) {
			defer wg.Done()
			similar, err := p.lastfm.GetSimilarTracks(ctx, s.Name, s.Artists[0], 10)
			if err != nil {
				return // Skip on error
			}

			for _, sim := range similar {
				spotifyTrack := p.searchOnSpotify(ctx, sim.Name, sim.Artist)
				if spotifyTrack != nil {
					mu.Lock()
					if !existingTrackIDs[spotifyTrack.ID] {
						candidates = append(candidates, *spotifyTrack)
					}
					mu.Unlock()
				}
			}
		}(seed)
	}
	wg.Wait()

	return p.deduplicateByID(candidates)
}

// scoreAndMerge scores and merges tag-based and similar-based candidates.
func (p *LastFmProvider) scoreAndMerge(tagCandidates, similarCandidates []track.Track) []ScoredTrack {
	scoreMap := make(map[string]*ScoredTrack)

	// Add tag candidates with tag weight
	for _, t := range tagCandidates {
		scoreMap[t.ID] = &ScoredTrack{
			Track: t,
			Score: p.config.TagWeight,
		}
	}

	// Add similar candidates (merge if already exists)
	for _, t := range similarCandidates {
		if existing, ok := scoreMap[t.ID]; ok {
			// Track found in both strategies - add similar weight
			existing.Score += p.config.SimilarWeight
		} else {
			// New track from similar strategy
			scoreMap[t.ID] = &ScoredTrack{
				Track: t,
				Score: p.config.SimilarWeight,
			}
		}
	}

	// Convert map to slice
	result := make([]ScoredTrack, 0, len(scoreMap))
	for _, scored := range scoreMap {
		result = append(result, *scored)
	}

	return result
}

// sortAndTakeTopTags sorts tags by count and returns top N tag names.
func (p *LastFmProvider) sortAndTakeTopTags(tagCounts map[string]int, topN int) []string {
	type tagCount struct {
		name  string
		count int
	}

	tags := make([]tagCount, 0, len(tagCounts))
	for name, count := range tagCounts {
		tags = append(tags, tagCount{name: name, count: count})
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i].count > tags[j].count
	})

	result := make([]string, 0, topN)
	for i := 0; i < topN && i < len(tags); i++ {
		result = append(result, tags[i].name)
	}

	return result
}

// searchOnSpotify searches for a track on Spotify with caching.
func (p *LastFmProvider) searchOnSpotify(ctx context.Context, trackName, artistName string) *track.Track {
	key := fmt.Sprintf("%s:%s", trackName, artistName)

	// Check cache
	p.cacheMutex.RLock()
	if cached, ok := p.spotifySearchCache[key]; ok {
		p.cacheMutex.RUnlock()
		return cached
	}
	p.cacheMutex.RUnlock()

	// Search on Spotify to get track ID
	query := fmt.Sprintf("track:%s artist:%s", trackName, artistName)
	results, err := p.spotify.Search(ctx, query, "track", 1)
	if err != nil || len(results) == 0 {
		// Cache nil to avoid repeated failed searches
		p.cacheMutex.Lock()
		p.spotifySearchCache[key] = nil
		p.cacheMutex.Unlock()
		return nil
	}

	// Get full track information including AvailableMarkets
	// Search API returns SimpleTrack without market information,
	// so we need to call GetTrack to get FullTrack with Markets
	trackID := results[0].ID
	fullTrack, err := p.spotify.GetTrack(ctx, trackID)
	if err != nil {
		// Cache nil to avoid repeated failed searches
		p.cacheMutex.Lock()
		p.spotifySearchCache[key] = nil
		p.cacheMutex.Unlock()
		return nil
	}

	// Cache result
	p.cacheMutex.Lock()
	p.spotifySearchCache[key] = fullTrack
	p.cacheMutex.Unlock()

	return fullTrack
}

// getChartBasedCandidates retrieves candidates using global chart strategy.
// This is used as a fallback when no seed tracks are available (e.g., at session start).
func (p *LastFmProvider) getChartBasedCandidates(ctx context.Context, count int, existingTrackIDs map[string]bool) ([]track.Track, error) {
	// Get chart top tracks (fetch more than needed for filtering)
	chartTracks, err := p.lastfm.GetChartTopTracks(ctx, 50)
	if err != nil {
		return []track.Track{}, err
	}

	// Initialize RNG
	var cryptoSeed int64
	var buf [8]byte
	if _, err := cryptoRand.Read(buf[:]); err == nil {
		cryptoSeed = int64(binary.LittleEndian.Uint64(buf[:]))
	} else {
		cryptoSeed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(cryptoSeed))

	// Shuffle chart tracks to avoid always picking the same top tracks
	// Shuffle multiple times for better randomness
	for i := 0; i < 3; i++ {
		rng.Shuffle(len(chartTracks), func(i, j int) {
			chartTracks[i], chartTracks[j] = chartTracks[j], chartTracks[i]
		})
	}

	var candidates []track.Track
	for _, chartTrack := range chartTracks {
		spotifyTrack := p.searchOnSpotify(ctx, chartTrack.Name, chartTrack.Artist)
		if spotifyTrack != nil && !existingTrackIDs[spotifyTrack.ID] {
			candidates = append(candidates, *spotifyTrack)
		}

		// Stop when we have enough candidates
		if len(candidates) >= count*2 {
			break
		}
	}

	return p.deduplicateByID(candidates), nil
}

// deduplicateByID removes duplicate tracks by ID.
func (p *LastFmProvider) deduplicateByID(tracks []track.Track) []track.Track {
	seen := make(map[string]bool)
	result := make([]track.Track, 0, len(tracks))

	for _, t := range tracks {
		if !seen[t.ID] {
			seen[t.ID] = true
			result = append(result, t)
		}
	}

	return result
}
