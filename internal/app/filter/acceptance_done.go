package filter

import (
	"context"
	"time"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// AcceptanceDoneFilter checks if the session is still accepting requests.
type AcceptanceDoneFilter struct {
	isAccepting      func() bool
	getEndTime       func() *time.Time
	getEndingDur     func() time.Duration
	getQueueDuration func() time.Duration
	getCurrentRemain func() time.Duration
	getNow           func() time.Time
}

// NewAcceptanceDoneFilter creates a new AcceptanceDoneFilter.
func NewAcceptanceDoneFilter(
	isAccepting func() bool,
	getEndTime func() *time.Time,
	getEndingDur func() time.Duration,
	getQueueDuration func() time.Duration,
	getCurrentRemain func() time.Duration,
	getNow func() time.Time,
) *AcceptanceDoneFilter {
	return &AcceptanceDoneFilter{
		isAccepting:      isAccepting,
		getEndTime:       getEndTime,
		getEndingDur:     getEndingDur,
		getQueueDuration: getQueueDuration,
		getCurrentRemain: getCurrentRemain,
		getNow:           getNow,
	}
}

func (f *AcceptanceDoneFilter) Name() string {
	return "acceptance_done_filter"
}

func (f *AcceptanceDoneFilter) Description() string {
	return "Checks if the session is still accepting requests"
}

func (f *AcceptanceDoneFilter) ReturnCodes() []string {
	return []string{"acceptance_done", "time_limit_exceeded"}
}

func (f *AcceptanceDoneFilter) ValidateConfig(settings map[string]any) error {
	return nil
}

func (f *AcceptanceDoneFilter) AppliesTo(requesterType track.RequesterType) bool {
	// This filter applies to user requests only
	// BGM selections are handled by queue depletion logic
	return requesterType == track.RequesterTypeUser
}

func (f *AcceptanceDoneFilter) Check(ctx context.Context, req TrackRequest, t track.Track, l *listener.Session) Result {
	// Check if session is accepting requests
	if !f.isAccepting() {
		return Reject("acceptance_done")
	}

	// Check time limit if end time is set
	endTime := f.getEndTime()
	if endTime != nil {
		endingDuration := f.getEndingDur()
		deadline := endTime.Add(-endingDuration)
		now := f.getNow()

		// First check: if current time has already passed the deadline, reject immediately
		// This prevents accepting requests even when queue is empty
		if now.After(deadline) {
			return Reject("time_limit_exceeded")
		}

		// Second check: calculate playback start time:
		// current time + current track remaining + queue duration
		// If the playback start time is at or after the deadline, reject the request
		// Note: This may allow the track to play beyond the deadline, but prevents
		// creating gaps in playback before the ending playlist
		currentRemaining := f.getCurrentRemain()
		queueDuration := f.getQueueDuration()
		playbackStartTime := now.Add(currentRemaining).Add(queueDuration)

		if playbackStartTime.After(deadline) || playbackStartTime.Equal(deadline) {
			return Reject("time_limit_exceeded")
		}
	}

	return Accept()
}

func init() {
	// Note: This filter requires runtime dependencies, so we register a placeholder.
	// The actual filter is created with dependencies in session manager.
}
