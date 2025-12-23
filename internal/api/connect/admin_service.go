// Package connect provides Connect RPC service implementations.
package connect

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/osa030/19box/internal/app/session"
	jukeboxv1 "github.com/osa030/19box/internal/gen/jukebox/v1"
	"github.com/osa030/19box/internal/gen/jukebox/v1/jukeboxv1connect"
	"github.com/osa030/19box/internal/infra/config"
)

// AdminService implements the AdminService RPC.
type AdminService struct {
	session *session.Manager
	config  *config.Config
}

// NewAdminService creates a new AdminService.
func NewAdminService(session *session.Manager, cfg *config.Config) *AdminService {
	return &AdminService{
		session: session,
		config:  cfg,
	}
}

// Ensure AdminService implements the interface.
var _ jukeboxv1connect.AdminServiceHandler = (*AdminService)(nil)

// GetStatus returns the current session status.
func (s *AdminService) GetStatus(
	ctx context.Context,
	req *connect.Request[jukeboxv1.GetStatusRequest],
) (*connect.Response[jukeboxv1.GetStatusResponse], error) {
	status := s.session.GetStatus()

	resp := &jukeboxv1.GetStatusResponse{
		QueueSize:     int32(status.QueueSize),
		ListenerCount: int32(status.ListenerCount),
		SessionInfo:   status.SessionInfo,
		CurrentTrack:  status.TrackInfo,
	}

	return connect.NewResponse(resp), nil
}

// Pause pauses the session.
func (s *AdminService) Pause(
	ctx context.Context,
	req *connect.Request[jukeboxv1.PauseRequest],
) (*connect.Response[jukeboxv1.PauseResponse], error) {
	err := s.session.Pause()
	if err != nil {
		return connect.NewResponse(&jukeboxv1.PauseResponse{
			Success: false,
			Message: err.Error(),
		}), nil
	}

	return connect.NewResponse(&jukeboxv1.PauseResponse{
		Success: true,
		Message: "Session paused",
	}), nil
}

// Resume resumes the session.
func (s *AdminService) Resume(
	ctx context.Context,
	req *connect.Request[jukeboxv1.ResumeRequest],
) (*connect.Response[jukeboxv1.ResumeResponse], error) {
	err := s.session.Resume()
	if err != nil {
		return connect.NewResponse(&jukeboxv1.ResumeResponse{
			Success: false,
			Message: err.Error(),
		}), nil
	}

	return connect.NewResponse(&jukeboxv1.ResumeResponse{
		Success: true,
		Message: "Session resumed",
	}), nil
}

// Skip skips the current track.
func (s *AdminService) Skip(
	ctx context.Context,
	req *connect.Request[jukeboxv1.SkipRequest],
) (*connect.Response[jukeboxv1.SkipResponse], error) {
	err := s.session.Skip()
	if err != nil {
		return connect.NewResponse(&jukeboxv1.SkipResponse{
			Success: false,
			Message: err.Error(),
		}), nil
	}

	return connect.NewResponse(&jukeboxv1.SkipResponse{
		Success: true,
		Message: "Track skipped",
	}), nil
}

// Kick kicks a listener.
func (s *AdminService) Kick(
	ctx context.Context,
	req *connect.Request[jukeboxv1.KickRequest],
) (*connect.Response[jukeboxv1.KickResponse], error) {
	err := s.session.KickListener(req.Msg.ListenerId)
	if err != nil {
		return connect.NewResponse(&jukeboxv1.KickResponse{
			Success: false,
			Message: err.Error(),
		}), nil
	}

	return connect.NewResponse(&jukeboxv1.KickResponse{
		Success: true,
		Message: "Listener kicked",
	}), nil
}

// ListListeners lists all listeners.
func (s *AdminService) ListListeners(
	ctx context.Context,
	req *connect.Request[jukeboxv1.ListListenersRequest],
) (*connect.Response[jukeboxv1.ListListenersResponse], error) {
	listeners := s.session.ListListeners()
	infos := make([]*jukeboxv1.ListenerInfo, len(listeners))

	for i, l := range listeners {
		infos[i] = &jukeboxv1.ListenerInfo{
			ListenerId:    l.ID,
			DisplayName:   l.DisplayName,
			PendingTracks: int32(l.PendingTracks),
			JoinedAt:      l.JoinedAt.Format(time.RFC3339),
			IsKicked:      l.IsKicked,
		}
	}

	return connect.NewResponse(&jukeboxv1.ListListenersResponse{
		Listeners: infos,
	}), nil
}

// StopSession stops the session.
func (s *AdminService) StopSession(
	ctx context.Context,
	req *connect.Request[jukeboxv1.StopSessionRequest],
) (*connect.Response[jukeboxv1.StopSessionResponse], error) {
	err := s.session.Stop(ctx)
	if err != nil {
		return connect.NewResponse(&jukeboxv1.StopSessionResponse{
			Success: false,
			Message: err.Error(),
		}), nil
	}

	return connect.NewResponse(&jukeboxv1.StopSessionResponse{
		Success: true,
		Message: "Session stopped",
	}), nil
}
