// Package connect provides Connect RPC service implementations.
package connect

import (
	"context"
	"sync"

	"connectrpc.com/connect"
	zlog "github.com/rs/zerolog/log"

	"github.com/osa030/19box/internal/app/session"
	jukeboxv1 "github.com/osa030/19box/internal/gen/jukebox/v1"
	"github.com/osa030/19box/internal/gen/jukebox/v1/jukeboxv1connect"
	"github.com/osa030/19box/internal/infra/config"
)

// ListenerService implements the ListenerService RPC.
type ListenerService struct {
	session *session.Manager
	config  *config.Config
}

// NewListenerService creates a new ListenerService.
func NewListenerService(session *session.Manager, cfg *config.Config) *ListenerService {
	return &ListenerService{
		session: session,
		config:  cfg,
	}
}

// Ensure ListenerService implements the interface.
var _ jukeboxv1connect.ListenerServiceHandler = (*ListenerService)(nil)

// Join handles listener join requests.
func (s *ListenerService) Join(
	ctx context.Context,
	req *connect.Request[jukeboxv1.JoinRequest],
) (*connect.Response[jukeboxv1.JoinResponse], error) {
	listenerID, err := s.session.Join(req.Msg.DisplayName, req.Msg.ExternalUserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&jukeboxv1.JoinResponse{
		ListenerId: listenerID,
	}), nil
}

// RequestTrack handles track request submissions.
func (s *ListenerService) RequestTrack(
	ctx context.Context,
	req *connect.Request[jukeboxv1.RequestTrackRequest],
) (*connect.Response[jukeboxv1.RequestTrackResponse], error) {
	success, code, err := s.session.RequestTrack(ctx, req.Msg.ListenerId, req.Msg.TrackId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var message string
	if success {
		message = s.config.GetMessage("success")
	} else {
		message = s.config.GetMessage(code)
	}
	return connect.NewResponse(&jukeboxv1.RequestTrackResponse{
		Success: success,
		Message: message,
		Code:    code,
	}), nil
}

// SubscribeNotifications handles notification subscription requests.
func (s *ListenerService) SubscribeNotifications(
	ctx context.Context,
	req *connect.Request[jukeboxv1.SubscribeNotificationsRequest],
	stream *connect.ServerStream[jukeboxv1.Notification],
) error {
	notifManager := s.session.GetNotificationManager()

	// 1. アダプターを用意し、購読を開始する
	// INITIAL_STATE送信前に届いた通知をバッファリングするため、Flushが必要
	adapter := &notificationStreamAdapter{stream: stream}
	subscriptionID, sequenceNo := notifManager.Subscribe(adapter)
	defer notifManager.Unsubscribe(subscriptionID)

	// 2. 現在の状態を取得
	status := s.session.GetStatus()

	// 3. 初期状態をNotificationとして構築 (sequenceNo は Subscribe 時のものを利用)
	initialNotification := &jukeboxv1.Notification{
		Type:        jukeboxv1.NotificationType_NOTIFICATION_TYPE_INITIAL_STATE,
		SequenceNo:  sequenceNo,
		SessionInfo: status.SessionInfo,
		TrackInfo:   status.TrackInfo,
	}

	// 4. 初期状態を送信
	zlog.Debug().Interface("notification", initialNotification).Msg("sending INITIAL_STATE notification")
	if err := stream.Send(initialNotification); err != nil {
		return err
	}

	// 5. バッファリングされていた通知をフラッシュし、それ以降は直接送信するようにする
	if err := adapter.Flush(); err != nil {
		return err
	}

	// Wait for context cancellation or session end
	select {
	case <-ctx.Done():
	case <-s.session.Done():
	}

	return nil
}

// notificationStreamAdapter adapts connect.ServerStream to notification.Stream.
type notificationStreamAdapter struct {
	mu     sync.Mutex
	stream *connect.ServerStream[jukeboxv1.Notification]
	buffer []*jukeboxv1.Notification
	ready  bool
}

func (a *notificationStreamAdapter) Send(notification *jukeboxv1.Notification) error {
	a.mu.Lock()
	if !a.ready {
		a.buffer = append(a.buffer, notification)
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()
	zlog.Debug().Interface("notification", notification).Msg("sending notification output")
	return a.stream.Send(notification)
}

func (a *notificationStreamAdapter) Flush() error {
	a.mu.Lock()
	a.ready = true
	buffer := a.buffer
	a.buffer = nil
	a.mu.Unlock()

	for _, n := range buffer {
		zlog.Debug().Interface("notification", n).Msg("sending notification output (flushed)")
		if err := a.stream.Send(n); err != nil {
			return err
		}
	}
	return nil
}
