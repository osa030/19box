package filter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

func TestDuplicateArtistFilter_ArtistInQueue(t *testing.T) {
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

	filter := NewDuplicateArtistFilter(qm)

	// Different track by same artist should be rejected
	result := filter.Check(
		context.Background(),
		TrackRequest{},
		track.Track{
			ID:      "track456",
			Name:    "We Will Rock You",
			Artists: []string{"Queen"},
		},
		&listener.Session{},
	)

	assert.False(t, result.Accepted)
	assert.Equal(t, "duplicate_artist", result.Code)
}

func TestDuplicateArtistFilter_DifferentArtist(t *testing.T) {
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

	filter := NewDuplicateArtistFilter(qm)

	// Track by different artist should be accepted
	result := filter.Check(
		context.Background(),
		TrackRequest{},
		track.Track{
			ID:      "track789",
			Name:    "Yesterday",
			Artists: []string{"The Beatles"},
		},
		&listener.Session{},
	)

	assert.True(t, result.Accepted)
}

func TestDuplicateArtistFilter_CaseInsensitive(t *testing.T) {
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

	filter := NewDuplicateArtistFilter(qm)

	// Same artist with different case should be rejected
	result := filter.Check(
		context.Background(),
		TrackRequest{},
		track.Track{
			ID:      "track456",
			Name:    "We Will Rock You",
			Artists: []string{"QUEEN"},
		},
		&listener.Session{},
	)

	assert.False(t, result.Accepted)
	assert.Equal(t, "duplicate_artist", result.Code)
}

func TestDuplicateArtistFilter_MultipleArtists(t *testing.T) {
	tests := []struct {
		name           string
		queuedArtists  []string
		requestArtists []string
		shouldReject   bool
		description    string
	}{
		{
			name:           "Featured artist matches",
			queuedArtists:  []string{"Queen"},
			requestArtists: []string{"David Bowie", "Queen"},
			shouldReject:   true,
			description:    "Should reject if any featured artist is in queue",
		},
		{
			name:           "Main artist matches",
			queuedArtists:  []string{"Queen", "David Bowie"},
			requestArtists: []string{"Queen"},
			shouldReject:   true,
			description:    "Should reject if main artist is in queue",
		},
		{
			name:           "Collaboration - no overlap",
			queuedArtists:  []string{"Queen", "David Bowie"},
			requestArtists: []string{"The Beatles", "Elton John"},
			shouldReject:   false,
			description:    "Should accept if no artist overlap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qm := &mockQueueManager{
				tracks: []track.QueuedTrack{
					{
						Track: track.Track{
							ID:      "track123",
							Name:    "Some Song",
							Artists: tt.queuedArtists,
						},
						Requester: track.Requester{ID: "user1", Name: "User 1"},
						AddedAt:   time.Now(),
					},
				},
			}

			filter := NewDuplicateArtistFilter(qm)
			result := filter.Check(
				context.Background(),
				TrackRequest{},
				track.Track{
					ID:      "track456",
					Name:    "Another Song",
					Artists: tt.requestArtists,
				},
				&listener.Session{},
			)

			if tt.shouldReject {
				assert.False(t, result.Accepted, tt.description)
				assert.Equal(t, "duplicate_artist", result.Code)
			} else {
				assert.True(t, result.Accepted, tt.description)
			}
		})
	}
}

func TestDuplicateArtistFilter_EmptyQueue(t *testing.T) {
	qm := &mockQueueManager{
		tracks: []track.QueuedTrack{},
	}

	filter := NewDuplicateArtistFilter(qm)

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

func TestDuplicateArtistFilter_VIPNotBypassed(t *testing.T) {
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

	filter := NewDuplicateArtistFilter(qm)

	// VIP should also be rejected (VIPStatus: true)
	result := filter.Check(
		context.Background(),
		TrackRequest{},
		track.Track{
			ID:      "track456",
			Name:    "We Will Rock You",
			Artists: []string{"Queen"},
		},
		&listener.Session{VIPStatus: true},
	)

	assert.False(t, result.Accepted, "VIP users should also be subject to this filter")
	assert.Equal(t, "duplicate_artist", result.Code)
}

func TestDuplicateArtistFilter_AppliesTo(t *testing.T) {
	filter := NewDuplicateArtistFilter(&mockQueueManager{})

	assert.True(t, filter.AppliesTo(track.RequesterTypeUser), "Should apply to user requests")
	assert.False(t, filter.AppliesTo(track.RequesterTypeBGM), "Should not apply to BGM")
	assert.False(t, filter.AppliesTo(track.RequesterTypeOpening), "Should not apply to opening")
	assert.False(t, filter.AppliesTo(track.RequesterTypeEnding), "Should not apply to ending")
}

func TestDuplicateArtistFilter_EndingPlaylist(t *testing.T) {
	tests := []struct {
		name           string
		endingTracks   []track.Track
		requestedTrack track.Track
		shouldReject   bool
		expectedCode   string
	}{
		{
			name: "Artist in ending playlist",
			endingTracks: []track.Track{
				{ID: "ending1", Name: "Final Song", Artists: []string{"Queen"}},
			},
			requestedTrack: track.Track{ID: "req1", Name: "Other Song", Artists: []string{"Queen"}},
			shouldReject:   true,
			expectedCode:   "reserved_artist",
		},
		{
			name: "Artist not in ending playlist",
			endingTracks: []track.Track{
				{ID: "ending1", Name: "Final Song", Artists: []string{"Queen"}},
			},
			requestedTrack: track.Track{ID: "req1", Name: "Yesterday", Artists: []string{"The Beatles"}},
			shouldReject:   false,
		},
		{
			name: "Featured artist in ending playlist",
			endingTracks: []track.Track{
				{ID: "ending1", Name: "Final Song", Artists: []string{"Queen", "David Bowie"}},
			},
			requestedTrack: track.Track{ID: "req1", Name: "Other Song", Artists: []string{"David Bowie"}},
			shouldReject:   true,
			expectedCode:   "reserved_artist",
		},
		{
			name: "Case insensitive match",
			endingTracks: []track.Track{
				{ID: "ending1", Name: "Final Song", Artists: []string{"Queen"}},
			},
			requestedTrack: track.Track{ID: "req1", Name: "Other Song", Artists: []string{"QUEEN"}},
			shouldReject:   true,
			expectedCode:   "reserved_artist",
		},
		{
			name:           "Empty ending playlist",
			endingTracks:   []track.Track{},
			requestedTrack: track.Track{ID: "req1", Name: "Any Song", Artists: []string{"Any Artist"}},
			shouldReject:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qm := &mockQueueManager{
				tracks:       []track.QueuedTrack{},
				endingTracks: tt.endingTracks,
			}

			filter := NewDuplicateArtistFilter(qm)
			result := filter.Check(
				context.Background(),
				TrackRequest{},
				tt.requestedTrack,
				&listener.Session{},
			)

			if tt.shouldReject {
				assert.False(t, result.Accepted, tt.name)
				assert.Equal(t, tt.expectedCode, result.Code)
			} else {
				assert.True(t, result.Accepted, tt.name)
			}
		})
	}
}
