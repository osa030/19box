package listener

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewSession(t *testing.T) {
	tests := []struct {
		name           string
		id             string
		displayName    string
		externalUserID string
		isVIP          bool
	}{
		{
			name:           "regular user",
			id:             "listener-1",
			displayName:    "Test User",
			externalUserID: "external-123",
			isVIP:          false,
		},
		{
			name:           "VIP user",
			id:             "listener-2",
			displayName:    "VIP User",
			externalUserID: "",
			isVIP:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := NewSession(tt.id, tt.displayName, tt.externalUserID, tt.isVIP)

			assert.Equal(t, tt.id, session.ID)
			assert.Equal(t, tt.displayName, session.DisplayName)
			assert.Equal(t, tt.externalUserID, session.ExternalUserID)
			assert.Equal(t, tt.isVIP, session.VIPStatus)
			assert.Equal(t, 0, session.PendingTracks)
			assert.False(t, session.IsKicked)
			assert.Equal(t, 0, session.TotalRequests)
			assert.Nil(t, session.LastRequestAt)
		})
	}
}

func TestSession_CanRequest(t *testing.T) {
	tests := []struct {
		name          string
		isKicked      bool
		vipStatus     bool
		pendingTracks int
		expected      bool
	}{
		{
			name:          "regular user, no pending",
			isKicked:      false,
			vipStatus:     false,
			pendingTracks: 0,
			expected:      true,
		},
		{
			name:          "regular user, has pending",
			isKicked:      false,
			vipStatus:     false,
			pendingTracks: 1,
			expected:      false,
		},
		{
			name:          "VIP user, no pending",
			isKicked:      false,
			vipStatus:     true,
			pendingTracks: 0,
			expected:      true,
		},
		{
			name:          "VIP user, has pending",
			isKicked:      false,
			vipStatus:     true,
			pendingTracks: 2,
			expected:      true,
		},
		{
			name:          "kicked regular user",
			isKicked:      true,
			vipStatus:     false,
			pendingTracks: 0,
			expected:      false,
		},
		{
			name:          "kicked VIP user",
			isKicked:      true,
			vipStatus:     true,
			pendingTracks: 0,
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &Session{
				ID:            "test-id",
				DisplayName:   "Test User",
				IsKicked:      tt.isKicked,
				VIPStatus:     tt.vipStatus,
				PendingTracks: tt.pendingTracks,
			}

			result := session.CanRequest()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSession_IncrementPendingTracks(t *testing.T) {
	session := NewSession("test-id", "Test User", "", false)

	assert.Equal(t, 0, session.PendingTracks)
	assert.Equal(t, 0, session.TotalRequests)
	assert.Nil(t, session.LastRequestAt)

	session.IncrementPendingTracks()

	assert.Equal(t, 1, session.PendingTracks)
	assert.Equal(t, 1, session.TotalRequests)
	assert.NotNil(t, session.LastRequestAt)

	firstRequestTime := session.LastRequestAt

	// Increment again
	time.Sleep(10 * time.Millisecond)
	session.IncrementPendingTracks()

	assert.Equal(t, 2, session.PendingTracks)
	assert.Equal(t, 2, session.TotalRequests)
	assert.True(t, session.LastRequestAt.After(*firstRequestTime))
}

func TestSession_DecrementPendingTracks(t *testing.T) {
	session := &Session{
		ID:            "test-id",
		DisplayName:   "Test User",
		PendingTracks: 3,
	}

	session.DecrementPendingTracks()
	assert.Equal(t, 2, session.PendingTracks)

	session.DecrementPendingTracks()
	assert.Equal(t, 1, session.PendingTracks)

	session.DecrementPendingTracks()
	assert.Equal(t, 0, session.PendingTracks)

	// Should not go below 0
	session.DecrementPendingTracks()
	assert.Equal(t, 0, session.PendingTracks)
}

func TestSession_Kick(t *testing.T) {
	session := NewSession("test-id", "Test User", "", false)

	assert.False(t, session.IsKicked)

	session.Kick()

	assert.True(t, session.IsKicked)
	assert.False(t, session.CanRequest())
}

func TestSession_PendingTracksWorkflow(t *testing.T) {
	// Simulate a complete workflow
	session := NewSession("test-id", "Regular User", "", false)

	// User can request initially
	assert.True(t, session.CanRequest())

	// User requests a track
	session.IncrementPendingTracks()
	assert.False(t, session.CanRequest(), "regular user cannot request when pending tracks exist")

	// Track starts playing
	session.DecrementPendingTracks()
	assert.True(t, session.CanRequest(), "regular user can request again after track starts")

	// VIP workflow
	vipSession := NewSession("vip-id", "VIP User", "", true)
	vipSession.IncrementPendingTracks()
	vipSession.IncrementPendingTracks()
	assert.True(t, vipSession.CanRequest(), "VIP can always request regardless of pending tracks")
}
