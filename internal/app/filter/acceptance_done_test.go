package filter

import (
	"context"
	"testing"
	"time"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
	"github.com/stretchr/testify/assert"
)

func TestAcceptanceDoneFilter_Check(t *testing.T) {
	now := time.Now()
	endTime := now.Add(1 * time.Hour)
	endingDuration := 5 * time.Minute

	tests := []struct {
		name             string
		isAccepting      bool
		endTime          *time.Time
		endingDuration   time.Duration
		queueDuration    time.Duration
		currentRemaining time.Duration
		trackDuration    time.Duration
		wantAccepted     bool
		wantCode         string
	}{
		{
			name:         "not accepting requests",
			isAccepting:  false,
			wantAccepted: false,
			wantCode:     "acceptance_done",
		},
		{
			name:         "no end time set",
			isAccepting:  true,
			endTime:      nil,
			wantAccepted: true,
		},
		{
			name:             "track fits before deadline",
			isAccepting:      true,
			endTime:          &endTime,
			endingDuration:   endingDuration,
			queueDuration:    10 * time.Minute,
			currentRemaining: 2 * time.Minute,
			trackDuration:    3 * time.Minute,
			// Deadline is 1 hour from now - 5 mins = 55 mins from now
			// Estimated end: 2 (current) + 10 (queue) + 3 (track) = 15 mins from now
			// 15 < 55 -> Accept
			wantAccepted: true,
		},
		{
			name:             "track start exceeds deadline",
			isAccepting:      true,
			endTime:          &endTime,
			endingDuration:   endingDuration,
			queueDuration:    50 * time.Minute,
			currentRemaining: 10 * time.Minute,
			trackDuration:    10 * time.Minute,
			// Deadline is 55 mins from now
			// Track start: 10 (current) + 50 (queue) = 60 mins from now
			// 60 > 55 -> Reject (start time exceeds deadline)
			wantAccepted: false,
			wantCode:     "time_limit_exceeded",
		},
		{
			name:             "track start exactly at deadline",
			isAccepting:      true,
			endTime:          &endTime,
			endingDuration:   endingDuration,
			queueDuration:    40 * time.Minute,
			currentRemaining: 15 * time.Minute,
			trackDuration:    10 * time.Minute,
			// Deadline is 55 mins from now
			// Track start: 15 (current) + 40 (queue) = 55 mins from now
			// 55 == 55 -> Reject (start time equals deadline)
			wantAccepted: false,
			wantCode:     "time_limit_exceeded",
		},
		{
			name:             "track end exceeds but start before deadline",
			isAccepting:      true,
			endTime:          &endTime,
			endingDuration:   endingDuration,
			queueDuration:    40 * time.Minute,
			currentRemaining: 10 * time.Minute,
			trackDuration:    10 * time.Minute,
			// Deadline is 55 mins from now
			// Track start: 10 (current) + 40 (queue) = 50 mins from now
			// Track end: 50 + 10 = 60 mins from now
			// Start is before deadline -> Accept (allows track to exceed deadline)
			wantAccepted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewAcceptanceDoneFilter(
				func() bool { return tt.isAccepting },
				func() *time.Time { return tt.endTime },
				func() time.Duration { return tt.endingDuration },
				func() time.Duration { return tt.queueDuration },
				func() time.Duration { return tt.currentRemaining },
				func() time.Time { return now },
			)

			trk := track.Track{
				ID:       "test-track",
				Duration: tt.trackDuration,
			}
			req := TrackRequest{TrackID: "test-track"}
			lis := &listener.Session{ID: "test-listener"}

			result := filter.Check(context.Background(), req, trk, lis)

			assert.Equal(t, tt.wantAccepted, result.Accepted, "accepted status mismatch")
			if !tt.wantAccepted {
				assert.Equal(t, tt.wantCode, result.Code, "rejection code mismatch")
			}
		})
	}
}
