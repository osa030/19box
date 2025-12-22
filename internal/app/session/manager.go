// Package session provides the session manager.
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"github.com/osa030/19box/internal/app/bgm"
	"github.com/osa030/19box/internal/app/filter"
	"github.com/osa030/19box/internal/app/notification"
	"github.com/osa030/19box/internal/app/playback"
	"github.com/osa030/19box/internal/app/session/registry"
	"github.com/osa030/19box/internal/app/session/state"
	"github.com/osa030/19box/internal/domain/listener"
	"github.com/osa030/19box/internal/domain/track"
	jukeboxv1 "github.com/osa030/19box/internal/gen/jukebox/v1"
	"github.com/osa030/19box/internal/infra/config"
	"github.com/osa030/19box/internal/infra/spotify"
	zlog "github.com/rs/zerolog/log"
)

var (
	ErrSessionNotRunning = errors.New("session is not running")
	ErrSessionNotPaused  = errors.New("session is not paused")
)

// Manager manages the jukebox session.
type Manager struct {
	mu sync.RWMutex

	// Configuration
	config *config.Config

	// Components
	stateMgr     *state.Manager
	listenerReg  *registry.ListenerRegistry
	playback     *playback.Controller
	filterChain  *filter.Chain
	notification *notification.Manager
	spotify      *spotify.Client

	// BGM provider
	bgmProvider *bgm.ProviderChain

	// Ending playlist config
	endingPlaylistURL string
	endingDisplayName string

	// System user for system-generated tracks
	systemUser *listener.Session

	// Artist deduplication for BGM
	recentArtists    []string
	maxRecentArtists int

	// Channels
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewManager creates a new session manager.
func NewManager(
	cfg *config.Config,
	spotifyClient *spotify.Client,
) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create BGM provider chain
	bgmProviderChain, err := bgm.NewProviderChainFromConfig(cfg, spotifyClient)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to create BGM provider chain")
	}

	sessionID := uuid.New().String()

	m := &Manager{
		config:      cfg,
		stateMgr:    state.New(sessionID),
		listenerReg: registry.NewListenerRegistry(),
	playback: playback.NewController(playback.Config{
			DepletionThresholdSec: cfg.BGM.DepletionThresholdSec,
			NotificationDelay:     time.Duration(cfg.Playback.NotificationDelayMs) * time.Millisecond,
			GapCorrection:         time.Duration(cfg.Playback.GapCorrectionMs) * time.Millisecond,
		}),
		spotify:      spotifyClient,
		notification: notification.NewManager(),
		filterChain:  filter.NewChain(),
		bgmProvider:  bgmProviderChain,

		endingPlaylistURL: cfg.Playlists.Ending.PlaylistURL,
		endingDisplayName: cfg.Playlists.Ending.DisplayName,

		recentArtists:    make([]string, 0),
		maxRecentArtists: cfg.BGM.RecentArtistCount,

		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	m.systemUser = listener.NewSession(
		uuid.New().String(),
		"System",
		"",
		false,
	)

	// Setup filters
	m.setupFilters()

	return m, nil
}

// setupFilters initializes the filter chain.
func (m *Manager) setupFilters() {
	cfg := m.config

	// AcceptanceDoneFilter
	m.filterChain.Add(filter.NewAcceptanceDoneFilter(
		func() bool { return m.stateMgr.CanAcceptRequests() },
		func() *time.Time { _, endTime := m.stateMgr.GetTimes(); return endTime },
		func() time.Duration { return m.stateMgr.GetEndingDuration() },
		func() time.Duration { return m.playback.GetTotalDuration() },
		func() time.Duration { return m.playback.GetRemainingDuration() },
		func() time.Time { return time.Now() },
	))

	// MarketFilter
	m.filterChain.Add(filter.NewMarketFilter(cfg.Spotify.Market))

	// KickedFilter
	if cfg.IsFilterEnabled("kicked_listener_filter") {
		m.filterChain.Add(&filter.KickedFilter{})
	}

	// UserPendingFilter
	if cfg.IsFilterEnabled("user_pending_filter") {
		m.filterChain.Add(&filter.UserPendingFilter{})
	}

	// DuplicateTrackFilter
	if cfg.IsFilterEnabled("duplicate_track_filter") {
		m.filterChain.Add(filter.NewDuplicateTrackFilter(m.playback))
	}

	// DurationLimitFilter
	if cfg.IsFilterEnabled("duration_limit_filter") {
		f := filter.NewDurationLimitFilter()
		settings := cfg.Filters["duration_limit_filter"].Settings
		if err := f.ValidateConfig(settings); err != nil {
			zlog.Error().Msgf("failed to validate duration limit filter config: %v", err)
		} else {
			m.filterChain.Add(f)
		}
	}
}

// Start starts the session.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()

	// Parse start time
	var startTime *time.Time
	if parsedStartTime, err := m.config.ParseStartTime(); err == nil && parsedStartTime != nil {
		startTime = parsedStartTime
	}

	// Parse end time
	var endTime *time.Time
	if parsedEndTime, err := m.config.ParseEndTime(); err == nil && parsedEndTime != nil {
		endTime = parsedEndTime
	}

	m.stateMgr.SetTimes(startTime, endTime)
	m.stateMgr.SetKeywords(m.config.Session.Keywords)

	// Wait for start time if needed
	if startTime != nil {
		now := time.Now()
		if now.Before(*startTime) {
			waitDuration := startTime.Sub(now)
			zlog.Info().Msgf("waiting for start time: start_time=%v wait_duration=%v", *startTime, waitDuration)
			m.mu.Unlock()

			select {
			case <-time.After(waitDuration):
			case <-ctx.Done():
				m.mu.Lock()
				return ctx.Err()
			}

			m.mu.Lock()
			zlog.Info().Msgf("start time reached, starting session: start_time=%v", *startTime)
		}
	}

	// Create session playlist
	createAt := time.Now().Format("2006-01-02 15:04")
	playlistName := m.config.Session.Title
	zlog.Info().Msgf("playlistName(config):[%s]", playlistName)
	if playlistName == "" {
		playlistName = fmt.Sprintf("Session(%s)", createAt)
	}
	zlog.Info().Msgf("playlistName(calculated):[%s]", playlistName)

	playlistID, err := m.spotify.CreatePlaylist(ctx, playlistName, "Created by 19box at "+createAt)
	if err != nil {
		m.mu.Unlock()
		return err
	}

	playlistURL := m.spotify.GetPlaylistURL(playlistID)
	m.stateMgr.SetPlaylistInfo(playlistID, playlistURL, playlistName)
	zlog.Debug().Msgf("playlist created: playlist_id=%s playlist_url=%s name=%s", playlistID, playlistURL, playlistName)

	// Load opening playlist
	if m.config.Playlists.Opening.PlaylistURL != "" {
		tracks, err := m.spotify.GetPlaylistTracks(ctx, m.config.Playlists.Opening.PlaylistURL)
		if err != nil {
			zlog.Error().Msgf("failed to load opening playlist: %v", err)
			m.mu.Unlock()
			return errors.Wrap(err, "failed to load opening playlist")
		}
		zlog.Info().Msgf("loaded opening playlist: track_count=%d", len(tracks))
		m.enqueuePlaylistTracks(tracks, m.config.Playlists.Opening.DisplayName, track.RequesterTypeOpening)
		trackIDs := make([]string, len(tracks))
		for i, t := range tracks {
			trackIDs[i] = t.ID
		}
		if err := m.spotify.AddTracksToPlaylist(ctx, playlistID, trackIDs); err != nil {
			zlog.Error().Msgf("failed to add opening tracks to session playlist: %v", err)
			m.mu.Unlock()
			return errors.Wrap(err, "failed to add opening tracks")
		}
	}

	// Calculate ending duration
	if m.endingPlaylistURL != "" {
		tracks, err := m.spotify.GetPlaylistTracks(ctx, m.endingPlaylistURL)
		if err != nil {
			zlog.Error().Msgf("failed to load ending playlist: %v", err)
			m.mu.Unlock()
			return errors.Wrap(err, "failed to load ending playlist")
		}
		var endingDuration time.Duration
		for _, t := range tracks {
			endingDuration += t.Duration
		}
		m.stateMgr.SetEndingDuration(endingDuration)
	}

	m.mu.Unlock()

	// If no opening playlist, fill queue with BGM before starting session
	if m.config.Playlists.Opening.PlaylistURL == "" {
		m.fillQueueWithBGM()
	}

	// Now that tracks are ready, transition to active phase
	m.mu.Lock()
	m.stateMgr.SetPhase(state.PhaseActive)
	m.stateMgr.StartAccepting()
	sessionID := m.stateMgr.GetSessionID()
	zlog.Info().Msgf("phase changed: phase=ACTIVE session_id=%s", sessionID)

	// Set startTime if not already set
	if startTime == nil {
		now := time.Now()
		m.stateMgr.SetTimes(&now, endTime)
	}
	m.mu.Unlock()

	// Broadcast session started
	sessionInfo := m.buildSessionInfoWithStateUnlocked()
	zlog.Info().Msgf("broadcast SESSION_STARTED: session_id=%s", sessionID)
	if err := m.notification.Broadcast(&jukeboxv1.Notification{
		Type:        jukeboxv1.NotificationType_NOTIFICATION_TYPE_CHANGE_STATE,
		SessionInfo: sessionInfo,
	}); err != nil {
		zlog.Error().Msgf("failed to broadcast SESSION_STARTED: %v", err)
	}

	// Start playback event loop
	go m.playbackLoop()

	// Start end time checker if needed
	if endTime != nil {
		go m.endTimeChecker()
	}

	// Start playing
	go func() {
		if err := m.playback.Play(); err != nil {
			zlog.Debug().Msgf("initial play: %v", err)
		}
	}()

	return nil
}

// Stop stops the session gracefully (plays ending playlist).
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()

	phase := m.stateMgr.GetPhase()
	if phase == state.PhaseTerminated {
		m.mu.Unlock()
		return nil
	}

	sessionID := m.stateMgr.GetSessionID()

	// PhaseWaiting: immediate termination
	if phase == state.PhaseWaiting {
		m.stateMgr.SetPhase(state.PhaseTerminated)
		zlog.Info().Msgf("phase changed: phase=TERMINATED session_id=%s reason=stopped_before_starting", sessionID)
		m.cancel()
		m.mu.Unlock()
		close(m.done)
		return nil
	}

	// PhaseEnding: already ending
	if phase == state.PhaseEnding {
		m.mu.Unlock()
		zlog.Info().Msgf("session already ending: session_id=%s", sessionID)
		return nil
	}

	// Active session: transition to ending
	zlog.Info().Msgf("stopping session gracefully: session_id=%s", sessionID)
	m.mu.Unlock()

	m.transitionToEnding("admin_stop")
	return nil
}

// StopImmediate immediately terminates the session.
func (m *Manager) StopImmediate(ctx context.Context) error {
	m.mu.Lock()

	phase := m.stateMgr.GetPhase()
	if phase == state.PhaseTerminated {
		m.mu.Unlock()
		return nil
	}

	sessionID := m.stateMgr.GetSessionID()

	if phase == state.PhaseWaiting {
		m.stateMgr.SetPhase(state.PhaseTerminated)
		zlog.Info().Msgf("phase changed: phase=TERMINATED session_id=%s", sessionID)
		m.cancel()
		m.mu.Unlock()
		close(m.done)
		return nil
	}

	if phase == state.PhaseEnding {
		m.mu.Unlock()
		return nil
	}

	m.stateMgr.SetPhase(state.PhaseTerminated)
	m.stateMgr.StopAccepting()

	actualEndTime := time.Now()
	startTime, _ := m.stateMgr.GetTimes()
	m.stateMgr.SetTimes(startTime, &actualEndTime)

	zlog.Info().Msgf("phase changed: phase=TERMINATED session_id=%s reason=immediate_stop", sessionID)

	_ = m.playback.Stop()
	m.mu.Unlock()

	// Broadcast SESSION_ENDED
	sessionInfo := m.buildSessionInfoWithStateUnlocked()
	zlog.Info().Msgf("broadcast SESSION_ENDED: session_id=%s", sessionID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := m.notification.Broadcast(&jukeboxv1.Notification{
			Type:        jukeboxv1.NotificationType_NOTIFICATION_TYPE_CHANGE_STATE,
			SessionInfo: sessionInfo,
		}); err != nil {
			zlog.Error().Msgf("failed to broadcast SESSION_ENDED: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		zlog.Warn().Msg("session ended notification timed out")
	}

	time.Sleep(100 * time.Millisecond)
	m.cancel()
	close(m.done)
	return nil
}

// transitionToEnding transitions the session to ending phase.
func (m *Manager) transitionToEnding(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stateMgr.GetPhase() != state.PhaseActive {
		return
	}

	m.stateMgr.StopAccepting()
	m.stateMgr.SetPhase(state.PhaseEnding)
	sessionID := m.stateMgr.GetSessionID()
	zlog.Info().Msgf("phase changed: phase=ENDING session_id=%s reason=%s", sessionID, reason)

	// Load ending playlist
	if m.endingPlaylistURL != "" {
		tracks, err := m.spotify.GetPlaylistTracks(context.Background(), m.endingPlaylistURL)
		if err != nil {
			zlog.Error().Msgf("failed to load ending playlist: %v", err)
			return
		}

		if len(tracks) == 0 {
			zlog.Warn().Msg("ending playlist is empty")
			return
		}

		// Clear queue and add ending tracks
		removed := m.playback.ClearQueue()
		zlog.Info().Msgf("removed unplayed tracks: count=%d", len(removed))

		// Remove from Spotify playlist
		if len(removed) > 0 {
			playlistID := m.stateMgr.GetPlaylistID()
			removedIDs := make([]string, len(removed))
			for i, qt := range removed {
				removedIDs[i] = qt.Track.ID
			}
			if err := m.spotify.RemoveTracksFromPlaylist(context.Background(), playlistID, removedIDs); err != nil {
				zlog.Error().Msgf("failed to remove tracks from playlist: %v", err)
			}
		}

		// Enqueue ending tracks
		m.enqueuePlaylistTracks(tracks, m.endingDisplayName, track.RequesterTypeEnding)

		// Add to Spotify playlist
		playlistID := m.stateMgr.GetPlaylistID()
		trackIDs := make([]string, len(tracks))
		for i, t := range tracks {
			trackIDs[i] = t.ID
		}
		if err := m.spotify.AddTracksToPlaylist(context.Background(), playlistID, trackIDs); err != nil {
			zlog.Error().Msgf("failed to add ending tracks to playlist: %v", err)
		}
	}
}

// terminateSession performs final termination.
func (m *Manager) terminateSession() {
	m.mu.Lock()

	if m.stateMgr.GetPhase() == state.PhaseTerminated {
		m.mu.Unlock()
		return
	}

	m.stateMgr.SetPhase(state.PhaseTerminated)
	m.stateMgr.StopAccepting()
	sessionID := m.stateMgr.GetSessionID()

	actualEndTime := time.Now()
	startTime, _ := m.stateMgr.GetTimes()
	m.stateMgr.SetTimes(startTime, &actualEndTime)

	zlog.Info().Msgf("phase changed: phase=TERMINATED session_id=%s", sessionID)

	_ = m.playback.Stop()
	m.mu.Unlock()

	// Broadcast SESSION_ENDED
	sessionInfo := m.buildSessionInfoWithStateUnlocked()
	zlog.Info().Msgf("broadcast SESSION_ENDED: session_id=%s", sessionID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := m.notification.Broadcast(&jukeboxv1.Notification{
			Type:        jukeboxv1.NotificationType_NOTIFICATION_TYPE_CHANGE_STATE,
			SessionInfo: sessionInfo,
		}); err != nil {
			zlog.Error().Msgf("failed to broadcast SESSION_ENDED: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		zlog.Warn().Msg("session ended notification timed out")
	}

	time.Sleep(100 * time.Millisecond)
	m.cancel()
	close(m.done)
}

// Done returns a channel that is closed when the session is stopped.
func (m *Manager) Done() <-chan struct{} {
	return m.done
}

// Pause pauses the session.
func (m *Manager) Pause() error {
	if m.stateMgr.GetPhase() != state.PhaseActive {
		return ErrSessionNotRunning
	}
	return m.playback.Pause()
}

// Resume resumes the session.
func (m *Manager) Resume() error {
	if m.playback.GetState() != playback.StatePaused {
		return ErrSessionNotPaused
	}
	return m.playback.Resume()
}

// Skip skips the current track.
func (m *Manager) Skip() error {
	return m.playback.Skip()
}

// Join adds a listener to the session.
func (m *Manager) Join(displayName, externalUserID string) (string, error) {
	if m.stateMgr.GetPhase() == state.PhaseTerminated {
		return "", ErrSessionNotRunning
	}

	isVIP := m.config.IsAdminDisplayName(displayName)
	id, err := m.listenerReg.Join(displayName, externalUserID, isVIP)
	if err != nil {
		return "", err
	}

	zlog.Info().Msgf("listener joined: listener_id=%s display_name=%s", id, displayName)
	return id, nil
}

// ValidateListener checks if a listener ID is valid.
func (m *Manager) ValidateListener(listenerID string) error {
	return m.listenerReg.Validate(listenerID)
}

// GetListenerSession returns a listener's session.
func (m *Manager) GetListenerSession(listenerID string) (*listener.Session, error) {
	return m.listenerReg.Get(listenerID)
}

// KickListener kicks a listener from the session.
func (m *Manager) KickListener(listenerID string) error {
	return m.listenerReg.Kick(listenerID)
}

// IncrementPendingTracks increments a listener's pending track count.
func (m *Manager) IncrementPendingTracks(listenerID string) error {
	return m.listenerReg.IncrementPending(listenerID)
}

// DecrementPendingTracks decrements a listener's pending track count.
func (m *Manager) DecrementPendingTracks(listenerID string) {
	m.listenerReg.DecrementPending(listenerID)
}

// RequestTrack handles a track request.
func (m *Manager) RequestTrack(ctx context.Context, listenerID, trackID string) (bool, string, error) {
	session, err := m.GetListenerSession(listenerID)
	if err != nil {
		zlog.Warn().Msgf("track request rejected: listener_id=%s code=invalid_listener", listenerID)
		return false, "invalid_listener", nil
	}

	t, err := m.spotify.GetTrack(ctx, trackID, m.config.Spotify.Market)
	if err != nil {
		zlog.Warn().Msgf("track request rejected: listener_id=%s track_id=%s code=track_not_found", listenerID, trackID)
		return false, "track_not_found", nil
	}

	req := filter.TrackRequest{
		ListenerID: listenerID,
		TrackID:    trackID,
	}
	result := m.filterChain.Execute(ctx, req, *t, session, track.RequesterTypeUser)
	zlog.Info().Msgf("track request: listener=%s track=%s result=%t code=%s", session.DisplayName, t.Name, result.Accepted, result.Code)
	if !result.Accepted {
		return false, result.Code, nil
	}

	qt := track.QueuedTrack{
		Track: *t,
		Requester: track.Requester{
			ID:             session.ID,
			Name:           session.DisplayName,
			ExternalUserID: session.ExternalUserID,
			Type:           track.RequesterTypeUser,
		},
		AddedAt: time.Now(),
	}
	m.playback.Enqueue(qt)
	m.addRecentArtists(t.Artists)

	if err := m.IncrementPendingTracks(listenerID); err != nil {
		zlog.Error().Msgf("failed to increment pending tracks: %v", err)
	}

	playlistID := m.stateMgr.GetPlaylistID()
	if err := m.spotify.AddTracksToPlaylist(ctx, playlistID, []string{trackID}); err != nil {
		zlog.Error().Msgf("failed to add track to playlist: %v", err)
	}

	// If playback is idle, start playing
	if m.playback.GetState() == playback.StateIdle {
		go func() {
			if err := m.playback.Play(); err != nil {
				zlog.Debug().Msgf("play after enqueue: %v", err)
			}
		}()
	}

	return true, "", nil
}

// Status represents the current session status with all information.
type Status struct {
	Phase         state.Phase
	PlaybackState playback.State
	CurrentTrack  *track.QueuedTrack
	Remaining     time.Duration
	QueueSize     int
	ListenerCount int
	SessionInfo   *jukeboxv1.SessionInfo
	TrackInfo     *jukeboxv1.TrackInfo
}

// GetStatus returns the current session status.
func (m *Manager) GetStatus() *Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	qt, _ := m.playback.GetCurrentTrack()
	remaining := m.playback.GetRemainingDuration()
	queueSize := m.playback.GetQueueSize()
	listenerCount := m.listenerReg.Count()
	pbState := m.playback.GetState()
	currentPhase := m.stateMgr.GetPhase()

	// SessionInfoを構築
	sessionInfo := m.buildSessionInfoWithStateUnlocked()

	// TrackInfoを構築
	var trackInfo *jukeboxv1.TrackInfo
	if qt != nil {
		trackInfo = m.buildTrackInfoWithState(qt, pbState)
	}

	return &Status{
		Phase:         currentPhase,
		PlaybackState: pbState,
		CurrentTrack:  qt,
		Remaining:     remaining,
		QueueSize:     queueSize,
		ListenerCount: listenerCount,
		SessionInfo:   sessionInfo,
		TrackInfo:     trackInfo,
	}
}

// ListListeners returns all listeners.
func (m *Manager) ListListeners() []*listener.Session {
	return m.listenerReg.All()
}

// GetNotificationManager returns the notification manager.
func (m *Manager) GetNotificationManager() *notification.Manager {
	return m.notification
}

// playbackLoop handles playback events.
func (m *Manager) playbackLoop() {
	defer func() {
		if r := recover(); r != nil {
			zlog.Error().Msgf("playback loop panicked: %v", r)
			// Restart loop to prevent zombie session
			zlog.Info().Msg("restarting playback loop")
			go m.playbackLoop()
		}
	}()

	for {
		select {
		case <-m.ctx.Done():
			return
		case event := <-m.playback.Events():
			m.handlePlaybackEvent(event)
		}
	}
}

// handlePlaybackEvent handles playback events.
func (m *Manager) handlePlaybackEvent(event playback.Event) {
	zlog.Info().Msgf("playback event: type=%s", event.Type)

	switch event.Type {
	case playback.EventTrackStarted:
		m.onTrackStarted(event.Track)

	case playback.EventTrackEnded:
		// Next track is automatically played by controller

	case playback.EventTrackSkipped:
		m.onTrackSkipped(event.Track)

	case playback.EventStateChanged:
		m.onStateChanged(event.State, event.Track)

	case playback.EventQueueDepleting:
		m.onQueueDepleting()

	case playback.EventQueueEmpty:
		m.onQueueEmpty()
	}
}

func (m *Manager) onTrackStarted(qt *track.QueuedTrack) {
	if qt == nil {
		return
	}

	m.DecrementPendingTracks(qt.Requester.ID)

	// SessionInfoを構築し、stateを設定
	sessionInfo := m.buildSessionInfoWithStateUnlocked()

	// TrackInfoを構築し、stateを設定
	pbState := m.playback.GetState()
	trackInfo := m.buildTrackInfoWithState(qt, pbState)
	trackInfo.State = jukeboxv1.TrackState_TRACK_STATE_STARTED

	zlog.Info().Msgf("broadcast TRACK_STARTED: track_id=%s name=%s", qt.Track.ID, qt.Track.Name)
	if err := m.notification.Broadcast(&jukeboxv1.Notification{
		Type:        jukeboxv1.NotificationType_NOTIFICATION_TYPE_CHANGE_TRACK,
		SessionInfo: sessionInfo,
		TrackInfo:   trackInfo,
	}); err != nil {
		zlog.Error().Msgf("failed to broadcast TRACK_STARTED: %v", err)
	}
}

func (m *Manager) onTrackSkipped(qt *track.QueuedTrack) {
	if qt == nil {
		return
	}

	// SessionInfoを構築し、stateを設定
	sessionInfo := m.buildSessionInfoWithStateUnlocked()

	// TrackInfoを構築し、stateを設定
	pbState := m.playback.GetState()
	trackInfo := m.buildTrackInfoWithState(qt, pbState)
	trackInfo.State = jukeboxv1.TrackState_TRACK_STATE_SKIPPED

	zlog.Info().Msgf("broadcast TRACK_SKIPPED: track_id=%s", qt.Track.ID)
	if err := m.notification.Broadcast(&jukeboxv1.Notification{
		Type:        jukeboxv1.NotificationType_NOTIFICATION_TYPE_CHANGE_TRACK,
		SessionInfo: sessionInfo,
		TrackInfo:   trackInfo,
	}); err != nil {
		zlog.Error().Msgf("failed to broadcast TRACK_SKIPPED: %v", err)
	}
}

func (m *Manager) onStateChanged(pbState playback.State, qt *track.QueuedTrack) {
	// SessionInfoを構築し、stateを設定
	sessionInfo := m.buildSessionInfoWithStateUnlocked()

	var logMsg string
	switch pbState {
	case playback.StatePaused:
		logMsg = "broadcast SESSION_PAUSED"
	case playback.StatePlaying:
		logMsg = "broadcast SESSION_RESUMED"
	default:
		return
	}
	zlog.Info().Msg(logMsg)

	var trackInfo *jukeboxv1.TrackInfo
	if qt != nil {
		trackInfo = m.buildTrackInfoWithState(qt, pbState)
	}

	if err := m.notification.Broadcast(&jukeboxv1.Notification{
		Type:        jukeboxv1.NotificationType_NOTIFICATION_TYPE_CHANGE_STATE,
		SessionInfo: sessionInfo,
		TrackInfo:   trackInfo,
	}); err != nil {
		zlog.Error().Msgf("failed to broadcast state change: %v", err)
	}
}

func (m *Manager) onQueueDepleting() {
	// Only fill queue if active and accepting
	if m.stateMgr.GetPhase() != state.PhaseActive || !m.stateMgr.IsAccepting() {
		return
	}

	m.fillQueueWithBGM()
}

func (m *Manager) onQueueEmpty() {
	phase := m.stateMgr.GetPhase()

	if phase == state.PhaseEnding {
		zlog.Info().Msg("ending playlist finished, terminating session")
		m.terminateSession()
		return
	}

	// If active, try to fill with BGM
	if phase == state.PhaseActive && m.stateMgr.IsAccepting() {
		m.fillQueueWithBGM()
	}
}

// fillQueueWithBGM fills the queue with BGM tracks.
func (m *Manager) fillQueueWithBGM() {
	if m.bgmProvider == nil {
		return
	}

	const maxRetries = 3

	// Get existing track IDs
	existingIDs := m.playback.GetAllTrackIDs()
	excludeSet := make(map[string]bool)
	for _, id := range existingIDs {
		excludeSet[id] = true
	}

	// Get seed tracks
	seedTracks := m.getRecentTracks(3)

	for retry := 0; retry < maxRetries; retry++ {
		candidates, err := m.bgmProvider.GetCandidates(context.Background(), 5, seedTracks, excludeSet)
		if err != nil {
			zlog.Error().Msgf("failed to get BGM candidates: %v", err)
			return
		}

		if len(candidates) == 0 {
			zlog.Warn().Msg("no BGM candidates")
			return
		}

		// Session state might have changed (e.g., ENDING) while fetching candidates.
		if m.stateMgr.GetPhase() != state.PhaseActive || !m.stateMgr.IsAccepting() {
			zlog.Debug().Msg("skipping BGM enqueue due to state change after candidate fetch")
			return
		}

		// Filter by recent artists
		filtered := m.filterByRecentArtists(candidates)


		// Process candidates
		for _, c := range filtered {
			// Check if queue became non-empty during selection (e.g. user added a track)
			if !m.playback.IsQueueEmpty() {
				zlog.Info().Msg("skipping BGM enqueue: queue is no longer empty")
				return
			}

			if m.playback.IsInQueue(c.Track.ID) {
				excludeSet[c.Track.ID] = true
				continue
			}

			req := filter.TrackRequest{
				ListenerID: m.systemUser.ID,
				TrackID:    c.Track.ID,
			}
			result := m.filterChain.Execute(context.Background(), req, c.Track, m.systemUser, track.RequesterTypeBGM)
			if !result.Accepted {
				zlog.Debug().Msgf("BGM candidate rejected by filter: track_id=%s name=%s reason=%s", c.Track.ID, c.Track.Name, result.Code)
				excludeSet[c.Track.ID] = true
				continue
			}

			qt := track.QueuedTrack{
				Track: c.Track,
				Requester: track.Requester{
					ID:   m.systemUser.ID,
					Name: c.DisplayName,
					Type: track.RequesterTypeBGM,
				},
				AddedAt: time.Now(),
			}
			m.playback.Enqueue(qt)

			playlistID := m.stateMgr.GetPlaylistID()
			if err := m.spotify.AddTracksToPlaylist(context.Background(), playlistID, []string{c.Track.ID}); err != nil {
				zlog.Error().Msgf("failed to add BGM to playlist: %v", err)
			}

			zlog.Info().Msgf("added BGM track: track_id=%s name=%s", c.Track.ID, c.Track.Name)
			m.addRecentArtists(c.Track.Artists)

			// If idle, start playing
			if m.playback.GetState() == playback.StateIdle {
				go func() {
					if err := m.playback.Play(); err != nil {
						zlog.Debug().Msgf("play after BGM: %v", err)
					}
				}()
			}

			// Add one track at a time
			return
		}

		// All candidates were filtered out, add them to exclude set and retry
		for _, c := range candidates {
			excludeSet[c.Track.ID] = true
		}
		zlog.Debug().Msgf("all BGM candidates filtered out, retrying: retry=%d/%d excluded_count=%d", retry+1, maxRetries, len(excludeSet))
	}

	zlog.Warn().Msg("no suitable BGM candidates after filtering")
}

func (m *Manager) filterByRecentArtists(candidates []bgm.CandidateWithSource) []bgm.CandidateWithSource {
	m.mu.Lock()
	defer m.mu.Unlock()

	zlog.Debug().Msgf("filterByRecentArtists: recent_artists=%v max=%d", m.recentArtists, m.maxRecentArtists)

	if m.maxRecentArtists == 0 {
		return candidates
	}

	var filtered []bgm.CandidateWithSource
	for _, c := range candidates {
		isRecent := m.isRecentArtistLocked(c.Track.Artists)
		zlog.Debug().Msgf("filterByRecentArtists: track=%s artists=%v is_recent=%v", c.Track.Name, c.Track.Artists, isRecent)
		if !isRecent {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 && len(m.recentArtists) > 0 {
		m.recentArtists = []string{}
		return candidates
	}

	return filtered
}

// Helper with no lock (caller must hold lock)
func (m *Manager) isRecentArtistLocked(artists []string) bool {
	for _, artist := range artists {
		for _, recent := range m.recentArtists {
			if artist == recent {
				return true
			}
		}
	}
	return false
}

func (m *Manager) addRecentArtists(artists []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addRecentArtistsLocked(artists)
}

func (m *Manager) addRecentArtistsLocked(artists []string) {
	for _, artist := range artists {
		m.recentArtists = append([]string{artist}, m.recentArtists...)
		if len(m.recentArtists) > m.maxRecentArtists {
			m.recentArtists = m.recentArtists[:m.maxRecentArtists]
		}
	}
}

func (m *Manager) getRecentTracks(count int) []track.Track {
	var recent []track.Track

	if qt, ok := m.playback.GetCurrentTrack(); ok {
		recent = append(recent, qt.Track)
	}

	if len(recent) > count {
		recent = recent[:count]
	}

	return recent
}

// endTimeChecker checks if the end time has been reached.
func (m *Manager) endTimeChecker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			_, endTime := m.stateMgr.GetTimes()
			phase := m.stateMgr.GetPhase()

			if endTime == nil || phase != state.PhaseActive {
				continue
			}

			endingDuration := m.stateMgr.GetEndingDuration()
			deadline := endTime.Add(-endingDuration)
			now := time.Now()

			if now.After(deadline) || now.Equal(deadline) {
				zlog.Info().Msgf("acceptance deadline reached: deadline=%v", deadline)
				m.transitionToEnding("acceptance_deadline_reached")
				return
			}
		}
	}
}

// enqueuePlaylistTracks enqueues tracks from a playlist.
func (m *Manager) enqueuePlaylistTracks(tracks []track.Track, requesterName string, requesterType track.RequesterType) {
	for _, t := range tracks {
		qt := track.QueuedTrack{
			Track: t,
			Requester: track.Requester{
				ID:   m.systemUser.ID,
				Name: requesterName,
				Type: requesterType,
			},
			AddedAt: time.Now(),
		}
		m.playback.Enqueue(qt)
		m.addRecentArtistsLocked(t.Artists)
	}
}

// Close closes the session manager.
func (m *Manager) Close() {
	m.cancel()
	m.playback.Close()
	m.notification.Close()
}

// buildTrackInfo creates a TrackInfo from a QueuedTrack.
func (m *Manager) buildTrackInfo(qt *track.QueuedTrack, remainingSeconds int32, trackURL string) *jukeboxv1.TrackInfo {
	if qt == nil {
		return nil
	}

	return &jukeboxv1.TrackInfo{
		TrackId:                 qt.Track.ID,
		Name:                    qt.Track.Name,
		Artists:                 qt.Track.Artists,
		Url:                     trackURL,
		AlbumArtUrl:             qt.Track.AlbumArtURL,
		RequesterName:           qt.Requester.Name,
		RequesterExternalUserId: qt.Requester.ExternalUserID,
		PlaylistUrl:             m.stateMgr.GetPlaylistURL(),
		RequesterType:           string(qt.Requester.Type),
		RemainingSeconds:        remainingSeconds,
		// Stateは呼び出し側で設定
	}
}

// buildSessionInfoWithStateUnlocked builds SessionInfo with state (internal use, no lock).
func (m *Manager) buildSessionInfoWithStateUnlocked() *jukeboxv1.SessionInfo {
	sessionInfo := m.stateMgr.BuildSessionInfo()
	currentPhase := m.stateMgr.GetPhase()
	pbState := m.playback.GetState()

	// Convert phase and playback state to protobuf state
	var protoState jukeboxv1.SessionState
	switch currentPhase {
	case state.PhaseWaiting:
		protoState = jukeboxv1.SessionState_SESSION_STATE_WAITING
	case state.PhaseActive:
		// Consider playback state for active phase
		switch pbState {
		case playback.StatePaused:
			protoState = jukeboxv1.SessionState_SESSION_STATE_PAUSED
		case playback.StateIdle:
			protoState = jukeboxv1.SessionState_SESSION_STATE_WAITING_FOR_TRACKS
		default:
			protoState = jukeboxv1.SessionState_SESSION_STATE_RUNNING
		}
	case state.PhaseEnding:
		protoState = jukeboxv1.SessionState_SESSION_STATE_ENDING
	case state.PhaseTerminated:
		protoState = jukeboxv1.SessionState_SESSION_STATE_TERMINATED
	default:
		protoState = jukeboxv1.SessionState_SESSION_STATE_UNSPECIFIED
	}

	sessionInfo.State = protoState
	return sessionInfo
}

// buildTrackInfoWithState builds TrackInfo with state (internal use, no lock).
func (m *Manager) buildTrackInfoWithState(qt *track.QueuedTrack, pbState playback.State) *jukeboxv1.TrackInfo {
	if qt == nil {
		return nil
	}

	remaining := m.playback.GetRemainingDuration()
	playlistID := m.stateMgr.GetPlaylistID()
	trackURLWithContext := m.spotify.GetTrackURLWithContext(qt.Track.ID, playlistID)

	trackInfo := m.buildTrackInfo(qt, int32(remaining.Seconds()), trackURLWithContext)

	// TrackInfo.stateを設定
	var trackState jukeboxv1.TrackState
	switch pbState {
	case playback.StatePaused:
		trackState = jukeboxv1.TrackState_TRACK_STATE_PAUSED
	case playback.StatePlaying:
		trackState = jukeboxv1.TrackState_TRACK_STATE_PLAYING
	default:
		// デフォルトはPLAYING
		trackState = jukeboxv1.TrackState_TRACK_STATE_PLAYING
	}
	trackInfo.State = trackState

	return trackInfo
}
