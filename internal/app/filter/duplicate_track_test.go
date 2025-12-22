package filter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// Mock QueueManager for testing
type mockQueueManager struct {
	tracks []track.QueuedTrack
}

func (m *mockQueueManager) GetAllTracks() []track.QueuedTrack {
	return m.tracks
}

func TestDuplicateTrackFilter_ExactIDMatch(t *testing.T) {
	qm := &mockQueueManager{
		tracks: []track.QueuedTrack{
			{
				Track: track.Track{
					ID:      "track123",
					Name:    "Bohemian Rhapsody",
					Artists: []string{"Queen"},
				},
				Requester: track.Requester{ID: "user1", Name: "User 1"},
				AddedAt:   time.Now(),
			},
		},
	}

	filter := NewDuplicateTrackFilter(qm)

	// Same track ID should be rejected
	result := filter.Check(
		context.Background(),
		TrackRequest{},
		track.Track{
			ID:      "track123",
			Name:    "Bohemian Rhapsody - 2011 Remaster",
			Artists: []string{"Queen"},
		},
		&listener.Session{},
	)

	assert.False(t, result.Accepted)
	assert.Equal(t, "duplicate_track", result.Code)
}

func TestDuplicateTrackFilter_RemasterDetection(t *testing.T) {
	tests := []struct {
		name           string
		queuedTrack    track.Track
		requestedTrack track.Track
		shouldReject   bool
		description    string
	}{
		{
			name: "Standard remaster pattern",
			queuedTrack: track.Track{
				ID:      "original123",
				Name:    "Bohemian Rhapsody",
				Artists: []string{"Queen"},
			},
			requestedTrack: track.Track{
				ID:      "remaster456",
				Name:    "Bohemian Rhapsody - 2011 Remaster",
				Artists: []string{"Queen"},
			},
			shouldReject: true,
			description:  "Should detect '- 2011 Remaster' as duplicate",
		},
		{
			name: "Remastered in parentheses",
			queuedTrack: track.Track{
				ID:      "original123",
				Name:    "Yesterday",
				Artists: []string{"The Beatles"},
			},
			requestedTrack: track.Track{
				ID:      "remaster456",
				Name:    "Yesterday (Remastered 2023)",
				Artists: []string{"The Beatles"},
			},
			shouldReject: true,
			description:  "Should detect '(Remastered 2023)' as duplicate",
		},
		{
			name: "Cover song - different artist",
			queuedTrack: track.Track{
				ID:      "original123",
				Name:    "Yesterday",
				Artists: []string{"The Beatles"},
			},
			requestedTrack: track.Track{
				ID:      "cover789",
				Name:    "Yesterday",
				Artists: []string{"Paul McCartney"},
			},
			shouldReject: false,
			description:  "Should allow cover by different artist",
		},
		{
			name: "Different songs - similar names",
			queuedTrack: track.Track{
				ID:      "track1",
				Name:    "Love",
				Artists: []string{"John Lennon"},
			},
			requestedTrack: track.Track{
				ID:      "track2",
				Name:    "Love Song",
				Artists: []string{"John Lennon"},
			},
			shouldReject: false,
			description:  "Should allow different songs",
		},
		{
			name: "Radio Edit version",
			queuedTrack: track.Track{
				ID:      "album123",
				Name:    "Stairway to Heaven",
				Artists: []string{"Led Zeppelin"},
			},
			requestedTrack: track.Track{
				ID:      "radio456",
				Name:    "Stairway to Heaven (Radio Edit)",
				Artists: []string{"Led Zeppelin"},
			},
			shouldReject: true,
			description:  "Should detect radio edit as duplicate",
		},
		{
			name: "Live version",
			queuedTrack: track.Track{
				ID:      "studio123",
				Name:    "Hotel California",
				Artists: []string{"Eagles"},
			},
			requestedTrack: track.Track{
				ID:      "live456",
				Name:    "Hotel California - Live",
				Artists: []string{"Eagles"},
			},
			shouldReject: true,
			description:  "Should detect live version as duplicate",
		},
		{
			name: "Multiple remasters in queue",
			queuedTrack: track.Track{
				ID:      "remaster2011",
				Name:    "Let It Be - 2011 Remaster",
				Artists: []string{"The Beatles"},
			},
			requestedTrack: track.Track{
				ID:      "remaster2023",
				Name:    "Let It Be (Remastered 2023)",
				Artists: []string{"The Beatles"},
			},
			shouldReject: true,
			description:  "Should detect different remasters as duplicate",
		},
		{
			name: "Remix version - should be allowed",
			queuedTrack: track.Track{
				ID:      "original123",
				Name:    "Le Freak",
				Artists: []string{"CHIC"},
			},
			requestedTrack: track.Track{
				ID:      "remix456",
				Name:    "Le Freak (Oliver Heldens Remix)",
				Artists: []string{"CHIC"},
			},
			shouldReject: false,
			description:  "Should allow remix version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qm := &mockQueueManager{
				tracks: []track.QueuedTrack{
					{
						Track:     tt.queuedTrack,
						Requester: track.Requester{ID: "user1", Name: "User 1"},
						AddedAt:   time.Now(),
					},
				},
			}

			filter := NewDuplicateTrackFilter(qm)
			result := filter.Check(
				context.Background(),
				TrackRequest{},
				tt.requestedTrack,
				&listener.Session{},
			)

			if tt.shouldReject {
				assert.False(t, result.Accepted, tt.description)
				assert.Equal(t, "duplicate_track", result.Code)
			} else {
				assert.True(t, result.Accepted, tt.description)
			}
		})
	}
}

func TestDuplicateTrackFilter_EmptyQueue(t *testing.T) {
	qm := &mockQueueManager{
		tracks: []track.QueuedTrack{},
	}

	filter := NewDuplicateTrackFilter(qm)

	result := filter.Check(
		context.Background(),
		TrackRequest{},
		track.Track{
			ID:      "track123",
			Name:    "Any Song",
			Artists: []string{"Any Artist"},
		},
		&listener.Session{},
	)

	assert.True(t, result.Accepted, "Should accept any track when queue is empty")
}

func TestDuplicateTrackFilter_AppliesTo(t *testing.T) {
	filter := NewDuplicateTrackFilter(&mockQueueManager{})

	assert.True(t, filter.AppliesTo(track.RequesterTypeUser), "Should apply to user requests")
	assert.False(t, filter.AppliesTo(track.RequesterTypeBGM), "Should not apply to BGM")
	assert.False(t, filter.AppliesTo(track.RequesterTypeOpening), "Should not apply to opening")
	assert.False(t, filter.AppliesTo(track.RequesterTypeEnding), "Should not apply to ending")
}

func TestNormalizeTrackName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Bohemian Rhapsody", "bohemian rhapsody"},
		{"Bohemian Rhapsody - 2011 Remaster", "bohemian rhapsody"},
		{"Yesterday (Remastered 2023)", "yesterday"},
		{"Hotel California [Remastered]", "hotel california"},
		{"Stairway to Heaven (Radio Edit)", "stairway to heaven"},
		{"Imagine - Live", "imagine"},
		{"Let It Be (Single Version)", "let it be"},
		{"Hey Jude - Remastered Version", "hey jude"},
		{"Come Together (2019 Mix)", "come together (2019 mix)"},
		{"   Extra   Spaces   ", "extra spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeTrackName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSameArtist(t *testing.T) {
	tests := []struct {
		name     string
		track1   track.Track
		track2   track.Track
		expected bool
	}{
		{
			name:     "Same artist",
			track1:   track.Track{Artists: []string{"Queen"}},
			track2:   track.Track{Artists: []string{"Queen"}},
			expected: true,
		},
		{
			name:     "Same artist - case insensitive",
			track1:   track.Track{Artists: []string{"Queen"}},
			track2:   track.Track{Artists: []string{"queen"}},
			expected: true,
		},
		{
			name:     "Different artists",
			track1:   track.Track{Artists: []string{"The Beatles"}},
			track2:   track.Track{Artists: []string{"Paul McCartney"}},
			expected: false,
		},
		{
			name:     "Empty artists array",
			track1:   track.Track{Artists: []string{}},
			track2:   track.Track{Artists: []string{"Queen"}},
			expected: false,
		},
		{
			name:     "Multiple artists - compare first",
			track1:   track.Track{Artists: []string{"Queen", "David Bowie"}},
			track2:   track.Track{Artists: []string{"Queen", "Someone Else"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSameArtist(tt.track1, tt.track2)
			assert.Equal(t, tt.expected, result)
		})
	}
}
