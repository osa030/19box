package filter

import (
	"context"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

// KickedFilter checks if the participant is kicked.
type KickedFilter struct{}

func (f *KickedFilter) Name() string {
	return "kicked_listener_filter"
}

func (f *KickedFilter) Description() string {
	return "Checks if the listener is kicked from the session"
}

func (f *KickedFilter) ReturnCodes() []string {
	return []string{"kicked"}
}

func (f *KickedFilter) ValidateConfig(settings map[string]any) error {
	return nil
}

func (f *KickedFilter) AppliesTo(requesterType track.RequesterType) bool {
	// Only user listeners can be kicked, system-generated tracks bypass this check
	return requesterType == track.RequesterTypeUser
}

func (f *KickedFilter) Check(ctx context.Context, req TrackRequest, t track.Track, l *listener.Session) Result {
	if l.IsKicked {
		return Reject("kicked")
	}
	return Accept()
}

func init() {
	Register("kicked_listener_filter", func() Filter {
		return &KickedFilter{}
	})
}
