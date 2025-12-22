// Package connect provides Connect RPC service implementations.
package connect

import (
	"context"

	"connectrpc.com/connect"

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
	// 1. シーケンス番号を取得
	notifManager := s.session.GetNotificationManager()
	sequenceNo := notifManager.NextSequenceNo()

	// 2. 現在の状態を取得
	status := s.session.GetStatus()

	// 3. 初期状態をNotificationとして構築
	initialNotification := &jukeboxv1.Notification{
		Type:        jukeboxv1.NotificationType_NOTIFICATION_TYPE_INITIAL_STATE,
		SequenceNo:  sequenceNo,
		SessionInfo: status.SessionInfo,
		TrackInfo:   status.TrackInfo,
	}

	// 4. 初期状態を送信
	if err := stream.Send(initialNotification); err != nil {
		return err
	}

	// 4. 通常の通知ストリームを開始
	adapter := &notificationStreamAdapter{stream: stream}
	subscriptionID := notifManager.Subscribe(adapter)

	// Wait for context cancellation or session end
	select {
	case <-ctx.Done():
	case <-s.session.Done():
	}

	// Unsubscribe when done
	notifManager.Unsubscribe(subscriptionID)

	return nil
}

// notificationStreamAdapter adapts connect.ServerStream to notification.Stream.
type notificationStreamAdapter struct {
	stream *connect.ServerStream[jukeboxv1.Notification]
}

func (a *notificationStreamAdapter) Send(notification *jukeboxv1.Notification) error {
	return a.stream.Send(notification)
}
