package playback

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/osa030/19box/internal/domain/track"
	zlog "github.com/rs/zerolog/log"
)

// Errors
var (
	ErrNoTrack    = errors.New("no track playing")
	ErrQueueEmpty = errors.New("queue is empty")
	ErrNotPlaying = errors.New("not playing")
	ErrNotPaused  = errors.New("not paused")
)

// Config holds controller configuration.
type Config struct {
	DepletionThresholdSec int           // Threshold for queue depletion warning
	NotificationDelay     time.Duration // Base delay before emitting EventTrackStarted
	GapCorrection         time.Duration // Small delay to compensate for client drift
}

// Controller manages playback with an internal queue.
type Controller struct {
	mu sync.RWMutex

	// Queue management
	queue  []track.QueuedTrack // Tracks waiting to be played
	played []track.QueuedTrack // Tracks that have been played (history)

	// Current track state
	currentTrack       *track.QueuedTrack
	state              State
	startTime          time.Time
	scheduledStartTime time.Time // Scheduled start time (before delay)
	notificationTime   time.Time // When the notification is scheduled to be emitted
	pausedAt           *time.Time
	pausedElapsed      time.Duration

	// Timer
	timerCancel                func() // Cancel function for track end timer
	depletionTimerCancel       func() // Cancel function for depletion timer
	notificationDelayTimerCancel func() // Cancel function for notification delay timer

	// Configuration
	config Config

	// Events
	eventCh chan Event

	// Context
	ctx    context.Context
	cancel context.CancelFunc

	// Depletion tracking
	depletionNotified bool
}

// NewController creates a new playback controller.
func NewController(config Config) *Controller {
	ctx, cancel := context.WithCancel(context.Background())
	return &Controller{
		queue:   make([]track.QueuedTrack, 0),
		played:  make([]track.QueuedTrack, 0),
		state:   StateIdle,
		config:  config,
		eventCh: make(chan Event, 10),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Events returns the event channel.
func (c *Controller) Events() <-chan Event {
	return c.eventCh
}

// Play starts playback. If a track is currently playing, it continues.
// If no track is playing, it dequeues and plays the next track.
// This is an explicit start action, so start delay is applied.
func (c *Controller) Play() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If already playing, do nothing
	if c.state == StatePlaying {
		return nil
	}

	// If paused, resume
	if c.state == StatePaused {
		return c.resumeLocked()
	}

	// If idle, start playing next track from queue
	// isContinuous=false because this is an explicit start
	return c.playNextLocked(false)
}

// Pause pauses the current playback.
func (c *Controller) Pause() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.currentTrack == nil {
		return ErrNoTrack
	}

	if c.state != StatePlaying {
		return ErrNotPlaying
	}

	// Stop timers
	if c.timerCancel != nil {
		c.timerCancel()
		c.timerCancel = nil
	}
	if c.depletionTimerCancel != nil {
		c.depletionTimerCancel()
		c.depletionTimerCancel = nil
	}
	if c.notificationDelayTimerCancel != nil {
		c.notificationDelayTimerCancel()
		c.notificationDelayTimerCancel = nil
	}

	now := toWallTime(time.Now())

	// If still in delay period, add remaining delay to pausedElapsed
	if now.Before(c.startTime) {
		c.pausedElapsed += c.startTime.Sub(now)
		c.startTime = now
	}

	c.pausedAt = &now
	c.state = StatePaused

	// Note: We don't need special handling for notificationTime during pause
	// because resumeLocked will calculate the remaining delay based on wall clock.
	// However, if we want to pause the delay itself, we'd need more logic.
	// For now, we'll keep it simple: notification delay is relative to track start.

	// Send event
	c.sendEventLocked(Event{
		Type:  EventStateChanged,
		Track: c.currentTrack,
		State: c.state,
	})

	return nil
}

// Resume resumes paused playback.
func (c *Controller) Resume() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.resumeLocked()
}

func (c *Controller) resumeLocked() error {
	if c.currentTrack == nil {
		return ErrNoTrack
	}

	if c.state != StatePaused {
		return ErrNotPaused
	}

	// Calculate elapsed time during pause
	if c.pausedAt != nil {
		c.pausedElapsed += time.Since(*c.pausedAt)
	}
	c.pausedAt = nil
	c.state = StatePlaying

	// Calculate remaining duration
	remaining := c.getRemainingDurationLocked()
	if remaining <= 0 {
		// Track should have ended, trigger end
		c.onTrackEndLocked()
		return nil
	}

	now := toWallTime(time.Now())

	// If notification is still pending, reschedule the delay timer
	if !c.notificationTime.IsZero() && now.Before(c.notificationTime) {
		delayRemaining := c.notificationTime.Sub(now)

		c.notificationDelayTimerCancel = c.startWallClockTimer(delayRemaining, func() {
			c.mu.Lock()
			defer c.mu.Unlock()

			// Clear the cancel func reference as it's already fired/done
			c.notificationDelayTimerCancel = nil

			if c.currentTrack == nil {
				return
			}

			// Send event
			c.sendEventLocked(Event{
				Type:  EventTrackStarted,
				Track: c.currentTrack,
				State: c.state,
			})
		})
	}

	// Restart track timer
	c.startTrackTimer(remaining)

	// Reschedule depletion check
	c.checkDepletionLocked()
	c.sendEventLocked(Event{
		Type:  EventStateChanged,
		Track: c.currentTrack,
		State: c.state,
	})

	return nil
}

// Skip skips the current track and plays the next one.
// This is a manual skip, so start delay is applied (new track start).
func (c *Controller) Skip() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.currentTrack == nil {
		return ErrNoTrack
	}

	// Stop timers
	if c.timerCancel != nil {
		c.timerCancel()
		c.timerCancel = nil
	}
	if c.notificationDelayTimerCancel != nil {
		c.notificationDelayTimerCancel()
		c.notificationDelayTimerCancel = nil
	}

	skippedTrack := c.currentTrack
	c.currentTrack = nil
	c.state = StateIdle
	c.pausedAt = nil
	c.pausedElapsed = 0
	c.notificationTime = time.Time{}

	// Send skip event
	c.sendEventLocked(Event{
		Type:  EventTrackSkipped,
		Track: skippedTrack,
		State: c.state,
	})

	// Play next track
	// isContinuous=false because this is a manual skip
	return c.playNextLocked(false)
}

// Stop stops playback completely.
func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.timerCancel != nil {
		c.timerCancel()
		c.timerCancel = nil
	}
	if c.depletionTimerCancel != nil {
		c.depletionTimerCancel()
		c.depletionTimerCancel = nil
	}

	c.currentTrack = nil
	c.state = StateIdle
	c.pausedAt = nil
	c.pausedElapsed = 0
	c.notificationTime = time.Time{}

	return nil
}

// Enqueue adds a track to the end of the queue.
func (c *Controller) Enqueue(qt track.QueuedTrack) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.queue = append(c.queue, qt)
	c.depletionNotified = false // Reset depletion flag when track is added
	c.checkDepletionLocked()    // Reschedule depletion timer
}

// EnqueueMultiple adds multiple tracks to the end of the queue.
func (c *Controller) EnqueueMultiple(qts []track.QueuedTrack) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.queue = append(c.queue, qts...)
	c.depletionNotified = false // Reset depletion flag when tracks are added
	c.checkDepletionLocked()    // Reschedule depletion timer
}

// ClearQueue removes all tracks from the queue.
func (c *Controller) ClearQueue() []track.QueuedTrack {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := c.queue
	c.queue = make([]track.QueuedTrack, 0)
	return removed
}

// GetState returns the current playback state.
func (c *Controller) GetState() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// GetCurrentTrack returns the currently playing track.
func (c *Controller) GetCurrentTrack() (*track.QueuedTrack, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.currentTrack == nil {
		return nil, false
	}
	return c.currentTrack, true
}

// GetRemainingDuration returns the remaining playback time.
func (c *Controller) GetRemainingDuration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getRemainingDurationLocked()
}

func (c *Controller) getRemainingDurationLocked() time.Duration {
	if c.currentTrack == nil {
		return 0
	}

	now := toWallTime(time.Now())

	// If still before actual start time (delay period), return full duration
	if now.Before(c.startTime) {
		return c.currentTrack.Track.Duration
	}

	elapsed := now.Sub(c.startTime) - c.pausedElapsed
	if c.state == StatePaused && c.pausedAt != nil {
		elapsed -= now.Sub(*c.pausedAt)
	}

	remaining := c.currentTrack.Track.Duration - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetQueueSize returns the number of tracks in the queue.
func (c *Controller) GetQueueSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.queue)
}

// IsQueueEmpty returns true if the queue is empty.
func (c *Controller) IsQueueEmpty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.queue) == 0
}

// GetAllTrackIDs returns all track IDs (played + queue).
func (c *Controller) GetAllTrackIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ids := make([]string, 0, len(c.played)+len(c.queue)+1)

	// Add played tracks
	for _, qt := range c.played {
		ids = append(ids, qt.Track.ID)
	}

	// Add current track
	if c.currentTrack != nil {
		ids = append(ids, c.currentTrack.Track.ID)
	}

	// Add queued tracks
	for _, qt := range c.queue {
		ids = append(ids, qt.Track.ID)
	}

	return ids
}

// IsInQueue checks if a track is in the queue.
func (c *Controller) IsInQueue(trackID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check current track
	if c.currentTrack != nil && c.currentTrack.Track.ID == trackID {
		return true
	}

	// Check queue
	for _, qt := range c.queue {
		if qt.Track.ID == trackID {
			return true
		}
	}

	return false
}

// GetQueuedTracks returns a copy of the queued tracks.
func (c *Controller) GetQueuedTracks() []track.QueuedTrack {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]track.QueuedTrack, len(c.queue))
	copy(result, c.queue)
	return result
}

// GetPlayedTracks returns a copy of the played tracks.
func (c *Controller) GetPlayedTracks() []track.QueuedTrack {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]track.QueuedTrack, len(c.played))
	copy(result, c.played)
	return result
}

// GetAllTracks returns all tracks (played + current + queued).
// Implements filter.QueueManager interface.
func (c *Controller) GetAllTracks() []track.QueuedTrack {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]track.QueuedTrack, 0, len(c.played)+len(c.queue)+1)
	result = append(result, c.played...)
	if c.currentTrack != nil {
		result = append(result, *c.currentTrack)
	}
	result = append(result, c.queue...)
	return result
}

// GetTotalDuration returns the total duration of all queued tracks.
func (c *Controller) GetTotalDuration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var total time.Duration
	for _, qt := range c.queue {
		total += qt.Track.Duration
	}
	return total
}

// Close closes the controller and releases resources.
func (c *Controller) Close() {
	c.cancel()
	_ = c.Stop()
	close(c.eventCh)
}

// playNextLocked plays the next track from the queue.
// isContinuous: true if this is a natural track update, false if explicit start/skip
// Must be called with lock held.
func (c *Controller) playNextLocked(isContinuous bool) error {
	if len(c.queue) == 0 {
		c.state = StateIdle
		c.sendEventLocked(Event{
			Type:  EventQueueEmpty,
			State: c.state,
		})
		return ErrQueueEmpty
	}

	// Dequeue next track
	qt := c.queue[0]
	c.queue = c.queue[1:]

	// Add to played history
	if c.currentTrack != nil {
		c.played = append(c.played, *c.currentTrack)
	}

	// Set current track
	c.currentTrack = &qt
	c.pausedAt = nil
	c.pausedElapsed = 0
	c.state = StatePlaying

	// Apply notification delay logic
	notificationDelay := c.config.NotificationDelay
	gapCorrection := c.config.GapCorrection

	// Always start the track timer and set start times immediately
	// Apply gap correction to scheduled start time
	startBase := toWallTime(time.Now())
	if gapCorrection > 0 {
		startBase = startBase.Add(gapCorrection)
	}

	c.scheduledStartTime = startBase
	c.startTime = c.scheduledStartTime

	// Set timer for track end
	// The track timer must account for the gap because during the gap, the track hasn't technically started playing on the client yet
	c.startTrackTimer(qt.Track.Duration + gapCorrection)

	// Check depletion
	c.checkDepletionLocked()

	if notificationDelay > 0 {
		c.notificationTime = c.scheduledStartTime.Add(notificationDelay)
		zlog.Debug().Msgf("playback: setting notification delay timer: delay=%v gap=%v track=%s duration=%v",
			notificationDelay, gapCorrection, qt.Track.Name, qt.Track.Duration)

		// Schedule delayed event emission
		// The notification delay is relative to the startTime (which already includes the gap)
		// So we wait for gap (to reach startTime) + notificationDelay
		totalDelay := notificationDelay + gapCorrection
		c.notificationDelayTimerCancel = c.startWallClockTimer(totalDelay, func() {
			c.mu.Lock()
			defer c.mu.Unlock()

			// Clear reference
			c.notificationDelayTimerCancel = nil

			// Check if track was skipped or changed during delay
			if c.currentTrack == nil || c.currentTrack.Track.ID != qt.Track.ID {
				return
			}

			zlog.Debug().Msgf("playback: notification delay completed, emitting EventTrackStarted: track=%s", qt.Track.Name)

			// Send event
			c.sendEventLocked(Event{
				Type:  EventTrackStarted,
				Track: c.currentTrack,
				State: c.state,
			})
		})
	} else {
		zlog.Debug().Msgf("playback: starting immediately (no notification delay): track=%s duration=%v",
			qt.Track.Name, qt.Track.Duration)

		// Send event immediately
		c.sendEventLocked(Event{
			Type:  EventTrackStarted,
			Track: c.currentTrack,
			State: c.state,
		})
	}

	return nil
}

// onTrackEnd is called when the current track ends.
func (c *Controller) onTrackEnd() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.onTrackEndLocked()
}

func (c *Controller) onTrackEndLocked() {
	if c.currentTrack == nil {
		return
	}

	// Stop any running timer
	if c.timerCancel != nil {
		c.timerCancel()
		c.timerCancel = nil
	}

	endedTrack := c.currentTrack

	if !c.startTime.IsZero() {
		elapsed := toWallTime(time.Now()).Sub(c.startTime)
		zlog.Debug().Msgf("playback: track ended: track=%s expected_duration=%v actual_elapsed=%v",
			endedTrack.Track.Name, endedTrack.Track.Duration, elapsed)
	}

	// Add to played history
	c.played = append(c.played, *endedTrack)

	// Clear current track
	c.currentTrack = nil
	c.pausedAt = nil
	c.pausedElapsed = 0

	// Send track ended event
	c.sendEventLocked(Event{
		Type:  EventTrackEnded,
		Track: endedTrack,
		State: c.state,
	})

	// Play next track
	// isContinuous=true because this is a natural track end
	_ = c.playNextLocked(true)
}

// checkDepletionLocked checks if queue is depleting and sends event.
// Must be called with lock held.
func (c *Controller) checkDepletionLocked() {
	if c.depletionNotified {
		return
	}

	// Cancel any existing depletion timer
	if c.depletionTimerCancel != nil {
		c.depletionTimerCancel()
		c.depletionTimerCancel = nil
	}

	// Calculate total remaining time: current track + all queued tracks
	var totalRemaining time.Duration

	// Add remaining time of current track
	if c.currentTrack != nil {
		totalRemaining = c.getRemainingDurationLocked()
	}

	// Add duration of all queued tracks
	for _, qt := range c.queue {
		totalRemaining += qt.Track.Duration
	}

	threshold := time.Duration(c.config.DepletionThresholdSec) * time.Second

	// Check if total remaining time is already below threshold
	if totalRemaining < threshold && totalRemaining > 0 {
		c.depletionNotified = true
		c.sendEventLocked(Event{
			Type:  EventQueueDepleting,
			Track: c.currentTrack,
			State: c.state,
		})
		return
	}

	// Schedule a timer to fire when remaining time drops below threshold
	if totalRemaining > threshold && c.state == StatePlaying {
		delay := totalRemaining - threshold
		c.depletionTimerCancel = c.startWallClockTimer(delay, func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			// Clear reference
			c.depletionTimerCancel = nil

			c.checkDepletionLocked()
		})
	}
}

// sendEventLocked sends an event without blocking.
// Must be called with lock held.
func (c *Controller) sendEventLocked(e Event) {
	select {
	case c.eventCh <- e:
		// Successfully sent
	case <-c.ctx.Done():
		// Context cancelled, don't send
	default:
		// Channel full, drop event (shouldn't happen with buffered channel)
	}
}

// startTrackTimer starts the track end timer using wall clock.
func (c *Controller) startTrackTimer(duration time.Duration) {
	if c.timerCancel != nil {
		c.timerCancel()
	}
	c.timerCancel = c.startWallClockTimer(duration, func() {
		c.onTrackEnd()
	})
}

// startWallClockTimer starts a timer that triggers callback after duration, using wall clock.
// Returns a cancel function.
func (c *Controller) startWallClockTimer(duration time.Duration, callback func()) func() {
	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Calculate wall-clock end time
	// Use manual wall clock calculation to avoid monotonic clock drift issues
	fn := func() {
		endTime := toWallTime(time.Now()).Add(duration)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if toWallTime(time.Now()).After(endTime) {
					callback()
					return
				}
			}
		}
	}

	go fn()

	return cancel
}

// toWallTime returns the time with monotonic clock stripped.
// This ensures that time differences are calculated using wall clock time,
// avoiding issues where the system monotonic clock runs faster/slower than real time.
func toWallTime(t time.Time) time.Time {
	return time.Unix(t.Unix(), int64(t.Nanosecond()))
}
