// Package main provides the user CLI entry point for testing.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"connectrpc.com/connect"
	"github.com/alecthomas/kingpin/v2"
	"github.com/joho/godotenv"

	jukeboxv1 "github.com/osa030/19box/internal/gen/jukebox/v1"
	"github.com/osa030/19box/internal/gen/jukebox/v1/jukeboxv1connect"
)

var (
	app    = kingpin.New("19box-usercli", "19box jukebox user client for testing")
	server = app.Flag("server", "Server address").Default("http://localhost:8080").String()

	// join command
	joinCmd        = app.Command("join", "Join the session")
	joinName       = joinCmd.Arg("name", "Display name").Required().String()
	joinExternalID = joinCmd.Arg("external-id", "External user ID (optional)").String()

	// request command
	requestCmd      = app.Command("request", "Request a track")
	requestListener = requestCmd.Arg("listener-id", "Listener ID (UUID)").Required().String()
	requestTrackID  = requestCmd.Arg("track-id", "Spotify track ID").Required().String()

	// subscribe command
	subscribeCmd = app.Command("subscribe", "Subscribe to notifications")
)

func main() {
	// Load .env file if it exists (errors are ignored)
	_ = godotenv.Load()

	// Parse command
	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	// Create client
	client := jukeboxv1connect.NewListenerServiceClient(
		http.DefaultClient,
		*server,
	)

	ctx := context.Background()

	// Execute command
	switch command {
	case joinCmd.FullCommand():
		join(ctx, client, *joinName, *joinExternalID)
	case requestCmd.FullCommand():
		requestTrack(ctx, client, *requestListener, *requestTrackID)
	case subscribeCmd.FullCommand():
		subscribe(ctx, client)
	}
}

func join(ctx context.Context, client jukeboxv1connect.ListenerServiceClient, displayName, externalID string) {
	resp, err := client.Join(ctx, connect.NewRequest(&jukeboxv1.JoinRequest{
		DisplayName:    displayName,
		ExternalUserId: externalID,
	}))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Joined! Your listener ID: %s\n", resp.Msg.ListenerId)
}

func requestTrack(ctx context.Context, client jukeboxv1connect.ListenerServiceClient, listenerID, trackID string) {
	resp, err := client.RequestTrack(ctx, connect.NewRequest(&jukeboxv1.RequestTrackRequest{
		ListenerId: listenerID,
		TrackId:    trackID,
	}))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if resp.Msg.Success {
		fmt.Printf("Success: %s\n", resp.Msg.Message)
	} else {
		fmt.Printf("Rejected [%s]: %s\n", resp.Msg.Code, resp.Msg.Message)
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

func subscribe(ctx context.Context, client jukeboxv1connect.ListenerServiceClient) {
	stream, err := client.SubscribeNotifications(ctx, connect.NewRequest(&jukeboxv1.SubscribeNotificationsRequest{}))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Subscribed to notifications. Press Ctrl+C to exit.")

	// Handle shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nUnsubscribing...")
		os.Exit(0)
	}()

	// Receive notifications
	for stream.Receive() {
		notification := stream.Msg()
		printNotification(notification)
	}

	if err := stream.Err(); err != nil {
		fmt.Printf("Stream error: %v\n", err)
	}
}

func printNotification(n *jukeboxv1.Notification) {
	// Print sequence number
	fmt.Printf("\n[Sequence: %d] ", n.SequenceNo)

	// Print event type header
	switch n.Type {
	case jukeboxv1.NotificationType_NOTIFICATION_TYPE_INITIAL_STATE:
		fmt.Println("=== INITIAL STATE ===")
	case jukeboxv1.NotificationType_NOTIFICATION_TYPE_CHANGE_STATE:
		fmt.Println("=== STATE CHANGED ===")
	case jukeboxv1.NotificationType_NOTIFICATION_TYPE_CHANGE_TRACK:
		fmt.Println("=== TRACK CHANGED ===")
	default:
		fmt.Printf("=== UNKNOWN EVENT (%v) ===\n", n.Type)
	}

	// Print SessionInfo if available
	if n.SessionInfo != nil {
		fmt.Println("\nSession Info:")
		fmt.Printf("  Session ID: %s\n", n.SessionInfo.SessionId)
		fmt.Printf("  Playlist Name: %s\n", n.SessionInfo.PlaylistName)
		fmt.Printf("  Playlist URL: %s\n", n.SessionInfo.PlaylistUrl)
		fmt.Printf("  Keywords: %v\n", n.SessionInfo.Keywords)
		fmt.Printf("  Scheduled Start Time: %s\n", n.SessionInfo.ScheduledStartTime)
		fmt.Printf("  Scheduled End Time: %s\n", n.SessionInfo.ScheduledEndTime)
		if n.SessionInfo.State != jukeboxv1.SessionState_SESSION_STATE_UNSPECIFIED {
			fmt.Printf("  State: %s\n", formatSessionState(n.SessionInfo.State))
		}
		fmt.Printf("  Accepting Requests: %v\n", n.SessionInfo.AcceptingRequests)
	}

	// Print TrackInfo if available
	if n.TrackInfo != nil {
		fmt.Println("\nTrack Info:")
		fmt.Printf("  Track ID: %s\n", n.TrackInfo.TrackId)
		fmt.Printf("  Name: %s\n", n.TrackInfo.Name)
		fmt.Printf("  Artists: %v\n", n.TrackInfo.Artists)
		fmt.Printf("  URL: %s\n", n.TrackInfo.Url)
		fmt.Printf("  Album Art URL: %s\n", n.TrackInfo.AlbumArtUrl)
		fmt.Printf("  Requested by: %s\n", n.TrackInfo.RequesterName)
		fmt.Printf("  Requester External User ID: %s\n", n.TrackInfo.RequesterExternalUserId)
		fmt.Printf("  Requester Type: %s\n", n.TrackInfo.RequesterType)
		fmt.Printf("  Playlist URL: %s\n", n.TrackInfo.PlaylistUrl)
		fmt.Printf("  Remaining: %d seconds\n", n.TrackInfo.RemainingSeconds)
		if n.TrackInfo.State != jukeboxv1.TrackState_TRACK_STATE_UNSPECIFIED {
			fmt.Printf("  Track State: %s\n", formatTrackState(n.TrackInfo.State))
		}
	}
	fmt.Println()
}
