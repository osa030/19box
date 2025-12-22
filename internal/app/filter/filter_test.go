package filter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
)

func TestMarketFilter_Check(t *testing.T) {
	tests := []struct {
		name         string
		filterMarket string
		trackMarkets []string
		wantAccepted bool
		wantCode     string
	}{
		{
			name:         "track available in market",
			filterMarket: "JP",
			trackMarkets: []string{"JP", "US", "UK"},
			wantAccepted: true,
		},
		{
			name:         "track not available in market",
			filterMarket: "JP",
			trackMarkets: []string{"US", "UK"},
			wantAccepted: false,
			wantCode:     "market_restriction",
		},
		{
			name:         "no market filter",
			filterMarket: "",
			trackMarkets: []string{"US"},
			wantAccepted: true,
		},
		{
			name:         "empty track markets",
			filterMarket: "JP",
			trackMarkets: []string{},
			wantAccepted: false,
			wantCode:     "market_restriction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &MarketFilter{market: tt.filterMarket}

			trk := track.Track{
				ID:      "test-track",
				Markets: tt.trackMarkets,
			}

			req := TrackRequest{TrackID: "test-track"}
			lis := &listener.Session{
				ID:          "test-listener",
				DisplayName: "Test User",
			}

			result := filter.Check(context.Background(), req, trk, lis)

			assert.Equal(t, tt.wantAccepted, result.Accepted,
				"MarketFilter.Check() accepted status mismatch")

			if !tt.wantAccepted {
				assert.Equal(t, tt.wantCode, result.Code,
					"MarketFilter.Check() rejection code mismatch")
			}
		})
	}
}

func TestUserPendingFilter_Check(t *testing.T) {
	tests := []struct {
		name          string
		pendingTracks int
		vipStatus     bool
		wantAccepted  bool
		wantCode      string
	}{
		{
			name:          "no pending tracks",
			pendingTracks: 0,
			vipStatus:     false,
			wantAccepted:  true,
		},
		{
			name:          "has pending tracks",
			pendingTracks: 1,
			vipStatus:     false,
			wantAccepted:  false,
			wantCode:      "user_pending",
		},
		{
			name:          "VIP user with pending tracks",
			pendingTracks: 2,
			vipStatus:     true,
			wantAccepted:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &UserPendingFilter{}

			trk := track.Track{ID: "test-track"}
			req := TrackRequest{TrackID: "test-track"}
			lis := &listener.Session{
				ID:            "test-listener",
				DisplayName:   "Test User",
				PendingTracks: tt.pendingTracks,
				VIPStatus:     tt.vipStatus,
			}

			result := filter.Check(context.Background(), req, trk, lis)

			assert.Equal(t, tt.wantAccepted, result.Accepted,
				"UserPendingFilter.Check() accepted status mismatch")

			if !tt.wantAccepted {
				assert.Equal(t, tt.wantCode, result.Code,
					"UserPendingFilter.Check() rejection code mismatch")
			}
		})
	}
}

func TestKickedFilter_Check(t *testing.T) {
	tests := []struct {
		name         string
		isKicked     bool
		wantAccepted bool
		wantCode     string
	}{
		{
			name:         "not kicked",
			isKicked:     false,
			wantAccepted: true,
		},
		{
			name:         "kicked user",
			isKicked:     true,
			wantAccepted: false,
			wantCode:     "kicked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &KickedFilter{}

			trk := track.Track{ID: "test-track"}
			req := TrackRequest{TrackID: "test-track"}
			lis := &listener.Session{
				ID:          "test-listener",
				DisplayName: "Test User",
				IsKicked:    tt.isKicked,
			}

			result := filter.Check(context.Background(), req, trk, lis)

			assert.Equal(t, tt.wantAccepted, result.Accepted)

			if !tt.wantAccepted {
				assert.Equal(t, tt.wantCode, result.Code)
			}
		})
	}
}
