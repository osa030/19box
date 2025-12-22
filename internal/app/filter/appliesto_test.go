package filter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/osa030/19box/internal/domain/track"
)

func TestMarketFilter_AppliesTo(t *testing.T) {
	filter := &MarketFilter{market: "JP"}

	tests := []struct {
		name          string
		requesterType track.RequesterType
		want          bool
	}{
		{"USER", track.RequesterTypeUser, true},
		{"SYSTEM", track.RequesterTypeSystem, true},
		{"OPENING", track.RequesterTypeOpening, true},
		{"ENDING", track.RequesterTypeEnding, true},
		{"BGM", track.RequesterTypeBGM, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.AppliesTo(tt.requesterType)
			assert.Equal(t, tt.want, result,
				"MarketFilter.AppliesTo() should apply to all requester types")
		})
	}
}

func TestUserPendingFilter_AppliesTo(t *testing.T) {
	filter := &UserPendingFilter{}

	tests := []struct {
		name          string
		requesterType track.RequesterType
		want          bool
	}{
		{"USER", track.RequesterTypeUser, true},
		{"SYSTEM", track.RequesterTypeSystem, false},
		{"OPENING", track.RequesterTypeOpening, false},
		{"ENDING", track.RequesterTypeEnding, false},
		{"BGM", track.RequesterTypeBGM, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.AppliesTo(tt.requesterType)
			assert.Equal(t, tt.want, result,
				"UserPendingFilter.AppliesTo() should only apply to USER type")
		})
	}
}

func TestKickedFilter_AppliesTo(t *testing.T) {
	filter := &KickedFilter{}

	tests := []struct {
		name          string
		requesterType track.RequesterType
		want          bool
	}{
		{"USER", track.RequesterTypeUser, true},
		{"SYSTEM", track.RequesterTypeSystem, false},
		{"OPENING", track.RequesterTypeOpening, false},
		{"ENDING", track.RequesterTypeEnding, false},
		{"BGM", track.RequesterTypeBGM, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.AppliesTo(tt.requesterType)
			assert.Equal(t, tt.want, result,
				"KickedFilter.AppliesTo() should only apply to USER type")
		})
	}
}

func TestAcceptanceDoneFilter_AppliesTo(t *testing.T) {
	filter := NewAcceptanceDoneFilter(
		func() bool { return true },
		func() *time.Time { return nil },
		func() time.Duration { return 0 },
		func() time.Duration { return 0 },
		func() time.Duration { return 0 },
		func() time.Time { return time.Now() },
	)

	tests := []struct {
		name          string
		requesterType track.RequesterType
		want          bool
	}{
		{"USER", track.RequesterTypeUser, true},
		{"SYSTEM", track.RequesterTypeSystem, false},
		{"OPENING", track.RequesterTypeOpening, false},
		{"ENDING", track.RequesterTypeEnding, false},

		{"BGM", track.RequesterTypeBGM, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.AppliesTo(tt.requesterType)
			assert.Equal(t, tt.want, result,
				"AcceptanceDoneFilter.AppliesTo() should only apply to USER type")
		})
	}
}
