package filter

import (
	"context"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// UserPendingFilter checks if the participant has pending tracks.
type UserPendingFilter struct{}

func (f *UserPendingFilter) Name() string {
	return "user_pending_filter"
}

func (f *UserPendingFilter) Description() string {
	return "Checks if the participant has tracks waiting to be played"
}

func (f *UserPendingFilter) ReturnCodes() []string {
	return []string{"user_pending"}
}

func (f *UserPendingFilter) ValidateConfig(settings map[string]any) error {
	return nil
}

func (f *UserPendingFilter) AppliesTo(requesterType track.RequesterType) bool {
	// Pending track limits only apply to user requests, not system-generated tracks
	return requesterType == track.RequesterTypeUser
}

func (f *UserPendingFilter) Check(ctx context.Context, req TrackRequest, t track.Track, l *listener.Session) Result {
	// VIP users bypass this check
	if l.VIPStatus {
		return Accept()
	}

	if l.PendingTracks > 0 {
		return Reject("user_pending")
	}
	return Accept()
}

func init() {
	Register("user_pending_filter", func() Filter {
		return &UserPendingFilter{}
	})
}
