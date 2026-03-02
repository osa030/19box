// Package main provides the Spotify authentication tool.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"golang.org/x/oauth2"
)

var (
	app          = kingpin.New("19box-auth", "Spotify authentication tool for 19box")
	clientID     = app.Flag("client-id", "Spotify Client ID").Envar("SPOTIFY_CLIENT_ID").Required().String()
	clientSecret = app.Flag("client-secret", "Spotify Client Secret").Envar("SPOTIFY_CLIENT_SECRET").Required().String()
	port         = app.Flag("port", "Callback server port").Default("8888").Int()

	oauthCfg *oauth2.Config
	ch        = make(chan *oauth2.Token)
	state     = "19box-auth-state"
)

func main() {
	// Parse flags
	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Build redirect URI with custom port
	customRedirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", *port)

	// Create OAuth2 config
	oauthCfg = &oauth2.Config{
		ClientID:     *clientID,
		ClientSecret: *clientSecret,
		RedirectURL:  customRedirectURI,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.spotify.com/authorize",
			TokenURL: "https://accounts.spotify.com/api/token",
		},
		Scopes: []string{
			"playlist-modify-public",
			"playlist-modify-private",
			"playlist-read-private",
		},
	}

	// Start HTTP server for callback
	http.HandleFunc("/callback", completeAuth)

	serverAddr := fmt.Sprintf(":%d", *port)
	server := &http.Server{Addr: serverAddr}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Print authorization URL
	url := oauthCfg.AuthCodeURL(state)
	fmt.Println("Please visit the following URL to authorize 19box:")
	fmt.Println("")
	fmt.Println(url)
	fmt.Println("")
	fmt.Println("Waiting for authorization...")

	// Wait for token
	token := <-ch

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Failed to shutdown server: %v", err)
	}

	// Print token
	fmt.Println("")
	fmt.Println("=== Authorization Successful ===")
	fmt.Println("")
	fmt.Println("Refresh Token:")
	fmt.Println(token.RefreshToken)
	fmt.Println("")
	fmt.Println("Add this to your config.yaml:")
	fmt.Println("")
	fmt.Println("spotify:")
	fmt.Printf("  refresh_token: \"%s\"\n", token.RefreshToken)
	fmt.Println("")
	fmt.Println("Or set as environment variable:")
	fmt.Printf("export SPOTIFY_REFRESH_TOKEN=\"%s\"\n", token.RefreshToken)
}

func completeAuth(w http.ResponseWriter, r *http.Request) {
	// Verify state before exchanging code
	if st := r.FormValue("state"); st != state {
		http.Error(w, "State mismatch", http.StatusForbidden)
		log.Printf("State mismatch: %s != %s", st, state)
		return
	}

	code := r.FormValue("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		log.Println("Missing authorization code in callback")
		return
	}

	token, err := oauthCfg.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "Failed to get token", http.StatusForbidden)
		log.Printf("Failed to get token: %v", err)
		return
	}

	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>19box - Authorization Complete</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #1DB954 0%%, #191414 100%%);
            color: white;
        }
        .container {
            text-align: center;
            padding: 40px;
            background: rgba(0, 0, 0, 0.5);
            border-radius: 16px;
        }
        h1 { margin-bottom: 20px; }
        p { opacity: 0.8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>✓ Authorization Complete</h1>
        <p>You can close this window and return to the terminal.</p>
    </div>
</body>
</html>
`)

	ch <- token
}
