// Package main provides the admin CLI entry point.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"github.com/alecthomas/kingpin/v2"
	"github.com/joho/godotenv"

	apiconnect "github.com/osa030/19box/internal/api/connect"
	jukeboxv1 "github.com/osa030/19box/internal/gen/jukebox/v1"
	"github.com/osa030/19box/internal/gen/jukebox/v1/jukeboxv1connect"
)

var (
	app    = kingpin.New("19box-admincli", "19box jukebox admin client")
	server = app.Flag("server", "Server address").Default("http://localhost:8080").String()
	token  = app.Flag("token", "Admin token (or set ADMIN_TOKEN env)").Envar("ADMIN_TOKEN").String()

	// status command
	statusCmd = app.Command("status", "Get session status")

	// pause command
	pauseCmd = app.Command("pause", "Pause the session")

	// resume command
	resumeCmd = app.Command("resume", "Resume the session")

	// skip command
	skipCmd = app.Command("skip", "Skip the current track")

	// kick command
	kickCmd      = app.Command("kick", "Kick a listener")
	kickListener = kickCmd.Arg("listener-id", "Listener ID (UUID)").Required().String()

	// list-listeners command
	listCmd = app.Command("list-listeners", "List all listeners").Alias("list")

	// stop command
	stopCmd = app.Command("stop", "Stop the session")
)

func main() {
	// Load .env file if it exists (errors are ignored)
	_ = godotenv.Load()

	// Parse command
	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	// Check admin token
	if *token == "" {
		fmt.Println("Error: admin token is required (use --token or ADMIN_TOKEN env)")
		os.Exit(1)
	}

	// Create client
	client := jukeboxv1connect.NewAdminServiceClient(
		http.DefaultClient,
		*server,
	)

	ctx := context.Background()

	// Execute command
	switch command {
	case statusCmd.FullCommand():
		status(ctx, client, *token)
	case pauseCmd.FullCommand():
		pause(ctx, client, *token)
	case resumeCmd.FullCommand():
		resume(ctx, client, *token)
	case skipCmd.FullCommand():
		skip(ctx, client, *token)
	case kickCmd.FullCommand():
		kick(ctx, client, *token, *kickListener)
	case listCmd.FullCommand():
		listListeners(ctx, client, *token)
	case stopCmd.FullCommand():
		stopSession(ctx, client, *token)
	}
}

func status(ctx context.Context, client jukeboxv1connect.AdminServiceClient, token string) {
	req := connect.NewRequest(&jukeboxv1.GetStatusRequest{})
	req.Header().Set(apiconnect.AdminTokenHeader, token)
	resp, err := client.GetStatus(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	s := resp.Msg
	fmt.Println("\n=== CURRENT SESSION STATUS ===")

	fmt.Printf("Queue Size: %d\n", s.QueueSize)
	fmt.Printf("Listeners: %d\n", s.ListenerCount)

	if s.SessionInfo != nil {
		fmt.Println("\nSession Info:")
		fmt.Printf("  Session ID: %s\n", s.SessionInfo.SessionId)
		fmt.Printf("  Playlist Name: %s\n", s.SessionInfo.PlaylistName)
		fmt.Printf("  Playlist URL: %s\n", s.SessionInfo.PlaylistUrl)
		fmt.Printf("  Keywords: %v\n", s.SessionInfo.Keywords)
		fmt.Printf("  Scheduled Start Time: %s\n", s.SessionInfo.ScheduledStartTime)
		fmt.Printf("  Scheduled End Time: %s\n", s.SessionInfo.ScheduledEndTime)
		if s.SessionInfo.State != jukeboxv1.SessionState_SESSION_STATE_UNSPECIFIED {
			fmt.Printf("  State: %s\n", formatSessionState(s.SessionInfo.State))
		}
		fmt.Printf("  Accepting Requests: %v\n", s.SessionInfo.AcceptingRequests)
	}

	if s.CurrentTrack != nil {
		fmt.Printf("\nCurrently Playing:\n")
		fmt.Printf("  Track ID: %s\n", s.CurrentTrack.TrackId)
		fmt.Printf("  Name: %s\n", s.CurrentTrack.Name)
		fmt.Printf("  Artists: %v\n", s.CurrentTrack.Artists)
		fmt.Printf("  URL: %s\n", s.CurrentTrack.Url)
		fmt.Printf("  Album Art URL: %s\n", s.CurrentTrack.AlbumArtUrl)
		fmt.Printf("  Requested by: %s\n", s.CurrentTrack.RequesterName)
		fmt.Printf("  Requester External User ID: %s\n", s.CurrentTrack.RequesterExternalUserId)
		fmt.Printf("  Requester Type: %s\n", s.CurrentTrack.RequesterType)
		fmt.Printf("  Playlist URL: %s\n", s.CurrentTrack.PlaylistUrl)
		fmt.Printf("  Remaining: %d seconds\n", s.CurrentTrack.RemainingSeconds)
		if s.CurrentTrack.State != jukeboxv1.TrackState_TRACK_STATE_UNSPECIFIED {
			fmt.Printf("  Track State: %s\n", formatTrackState(s.CurrentTrack.State))
		}
	} else {
		fmt.Println("\nNo track currently playing")
	}
	fmt.Println()
}

func pause(ctx context.Context, client jukeboxv1connect.AdminServiceClient, token string) {
	req := connect.NewRequest(&jukeboxv1.PauseRequest{})
	req.Header().Set(apiconnect.AdminTokenHeader, token)
	resp, err := client.Pause(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if resp.Msg.Success {
		fmt.Println("Session paused")
	} else {
		fmt.Printf("Failed: %s\n", resp.Msg.Message)
	}
}

func resume(ctx context.Context, client jukeboxv1connect.AdminServiceClient, token string) {
	req := connect.NewRequest(&jukeboxv1.ResumeRequest{})
	req.Header().Set(apiconnect.AdminTokenHeader, token)
	resp, err := client.Resume(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if resp.Msg.Success {
		fmt.Println("Session resumed")
	} else {
		fmt.Printf("Failed: %s\n", resp.Msg.Message)
	}
}

func skip(ctx context.Context, client jukeboxv1connect.AdminServiceClient, token string) {
	req := connect.NewRequest(&jukeboxv1.SkipRequest{})
	req.Header().Set(apiconnect.AdminTokenHeader, token)
	resp, err := client.Skip(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if resp.Msg.Success {
		fmt.Println("Track skipped")
	} else {
		fmt.Printf("Failed: %s\n", resp.Msg.Message)
	}
}

func kick(ctx context.Context, client jukeboxv1connect.AdminServiceClient, token, listenerID string) {
	req := connect.NewRequest(&jukeboxv1.KickRequest{
		ListenerId: listenerID,
	})
	req.Header().Set(apiconnect.AdminTokenHeader, token)
	resp, err := client.Kick(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if resp.Msg.Success {
		fmt.Println("Listener kicked")
	} else {
		fmt.Printf("Failed: %s\n", resp.Msg.Message)
	}
}

func listListeners(ctx context.Context, client jukeboxv1connect.AdminServiceClient, token string) {
	req := connect.NewRequest(&jukeboxv1.ListListenersRequest{})
	req.Header().Set(apiconnect.AdminTokenHeader, token)
	resp, err := client.ListListeners(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Listeners (%d):\n", len(resp.Msg.Listeners))
	for _, l := range resp.Msg.Listeners {
		displayName := l.DisplayName
		if l.IsKicked {
			displayName = "[KICKED] " + displayName
		}
		fmt.Printf("  %s: %s (pending: %d, joined: %s)\n",
			l.ListenerId, displayName, l.PendingTracks, l.JoinedAt)
	}
}

func stopSession(ctx context.Context, client jukeboxv1connect.AdminServiceClient, token string) {
	req := connect.NewRequest(&jukeboxv1.StopSessionRequest{})
	req.Header().Set(apiconnect.AdminTokenHeader, token)
	resp, err := client.StopSession(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if resp.Msg.Success {
		fmt.Println("Session stopped")
	} else {
		fmt.Printf("Failed: %s\n", resp.Msg.Message)
	}
}

func formatSessionState(state jukeboxv1.SessionState) string {
	switch state {
	case jukeboxv1.SessionState_SESSION_STATE_WAITING:
		return "⏳ Waiting (not started yet)"
	case jukeboxv1.SessionState_SESSION_STATE_RUNNING:
		return "▶️  Running"
	case jukeboxv1.SessionState_SESSION_STATE_PAUSED:
		return "⏸  Paused"
	case jukeboxv1.SessionState_SESSION_STATE_WAITING_FOR_TRACKS:
		return "⏸  Waiting for tracks (queue empty, auto-resume when track added)"
	case jukeboxv1.SessionState_SESSION_STATE_ENDING:
		return "🔚 Ending (playing final tracks)"
	case jukeboxv1.SessionState_SESSION_STATE_TERMINATED:
		return "⏹  Terminated"
	default:
		return "❓ Unknown"
	}
}

func formatTrackState(state jukeboxv1.TrackState) string {
	switch state {
	case jukeboxv1.TrackState_TRACK_STATE_STARTED:
		return "▶️  Started"
	case jukeboxv1.TrackState_TRACK_STATE_SKIPPED:
		return "⏭  Skipped"
	case jukeboxv1.TrackState_TRACK_STATE_PLAYING:
		return "▶️  Playing"
	case jukeboxv1.TrackState_TRACK_STATE_PAUSED:
		return "⏸  Paused"
	default:
		return "❓ Unknown"
	}
}
