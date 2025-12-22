# 19box

19box (pronounced "jukebox") is an online participatory jukebox system powered by Spotify.

Participants (listeners) can request Spotify tracks during an active session.
Requested tracks are queued in order, and listeners are notified when playback begins. Accepted tracks are saved to a public Spotify playlist for each session, allowing listeners to check the playlist when notified and experience the music as if listening together in real-time.

## Features

- **Session Management**: Time-based or manual session control with configurable start/end times
- **Track Requests**: Users can request Spotify tracks via gRPC API
- **Smart BGM**: Automatic background music selection using Last.fm recommendations or Spotify playlists
- **Filters**: Built-in filters for duplicate tracks, pending requests, and kicked users
- **Opening/Ending Playlists**: Automated session intro and outro music
- **Market Restrictions**: Automatic handling of region-restricted content
- **Real-time Queue Management**: Dynamic queue with automatic track selection when depleted

## Architecture

The system consists of multiple components:

- **server**: Core gRPC server handling requests and Spotify integration
- **admincli**: Command-line tool for administrative tasks
- **usercli**: Command-line tool for user track requests
- **auth**: Authentication service for Spotify OAuth flow

## Prerequisites

- Go 1.25.0 or later
- [Buf](https://buf.build/) for Protocol Buffer generation
- Spotify Premium account
- Spotify Developer Application (Client ID & Secret)
- Last.fm API key (optional, for smart BGM)

## Supported Platforms
- Linux
- Windows,macOS (Maybe, but I haven't tried it)
## Installation

1. Clone the repository:
```bash
git clone https://github.com/osa030/19box.git
cd 19box
```

2. Install dependencies:
```bash
go mod tidy
```

```bash
./build.sh
```

Or build manually:
```bash
go build -o bin/19box-server ./cmd/server
go build -o bin/19box-admincli ./cmd/admincli
go build -o bin/19box-usercli ./cmd/usercli
go build -o bin/19box-auth ./cmd/auth
```

The binaries will be generated in the `bin/` directory:
- `bin/19box-server`
- `bin/19box-admincli`
- `bin/19box-usercli`
- `bin/19box-auth`

## Configuration

1. Copy the base configuration:
```bash
cp config/server.yaml.example config/server.yaml
```

2. Edit `config/server.yaml` and configure:
   - Session settings (title, start/end times, keywords)
   - Admin token for API authentication
   - Spotify API settings (credentials, market)
   - Spotify playlists (opening, ending, BGM)
   - Filter settings
   - Custom messages
   - Logging preferences

3. Set environment variables (recommended for sensitive data):
```bash
export ADMIN_TOKEN="your-admin-token"
export SPOTIFY_CLIENT_ID="your-client-id"
export SPOTIFY_CLIENT_SECRET="your-client-secret"
export SPOTIFY_REFRESH_TOKEN="your-refresh-token"
export LASTFM_API_KEY="your-lastfm-api-key" # Optional
```

Alternatively, create a `.env` file in the project root.

## Spotify Setup

1. Create a Spotify Developer Application at https://developer.spotify.com/dashboard
2. Add "http://127.0.0.1:8888/callback" to "Redirect URIs" (ensure the port matches 19box-auth)
3. Note your Client ID and Client Secret
4. Use the `auth` tool to obtain a refresh token:
```bash
bin/19box-auth --client-id YOUR_CLIENT_ID --client-secret YOUR_CLIENT_SECRET
```
5. Follow the OAuth flow in your browser
6. Save the refresh token to your configuration

## Usage

### Running the Server

```bash
bin/19box-server
```

The server will start listening for gRPC connections (default: port 8080).

### Using the Admin CLI

```bash
# Get session status
bin/19box-admincli status

# List listeners
bin/19box-admincli list

# Pause playback
bin/19box-admincli pause

# Resume playback
bin/19box-admincli resume

# Skip current track
bin/19box-admincli skip

# Kick a listener
bin/19box-admincli kick <listener-id>

# Stop the session
bin/19box-admincli stop
```

### Using the User CLI

```bash
# Join session (returns listener ID)
bin/19box-usercli join <display-name> [external-id]

# Request a track
bin/19box-usercli request <listener-id> <spotify-track-id>

# Subscribe to notifications
bin/19box-usercli subscribe
```
#### spotify-track-id
Any of the following formats are accepted:
- **Spotify Track URL 1**: https://open.spotify.com/track/0ee1DiZF94NSqqpG0XHUzH
- **Spotify Track URL 2**: https://open.spotify.com/intl-xx/track/0ee1DiZF94NSqqpG0XHUzH
- **Spotify Track URI**:spotify:track:0ee1DiZF94NSqqpG0XHUzH
- **Spotify Track ID**:0ee1DiZF94NSqqpG0XHUzH


## Configuration Reference

### Spotify Settings

- `client_id`: Spotify Client ID
- `client_secret`: Spotify Client Secret
- `refresh_token`: Spotify Refresh Token
- `market`: ISO 3166-1 alpha-2 country code for track availability check (default: "JP")

### Session Settings

- `title`: Session name displayed to users (also used as Spotify playlist name)
- `start_time`: ISO 8601 timestamp (empty = start immediately)
- `end_time`: ISO 8601 timestamp (empty = manual end only)
- `keywords`: Optional theme keywords for the session (used for notifications)

### Playlist Settings

- `opening`: Configuration for opening playlist (played at session start)
  - `playlist_url`: Spotify playlist URL or URI
  - `display_name`: Name displayed as the requester
- `ending`: Configuration for ending playlist (played at session end)
  - `playlist_url`: Spotify playlist URL or URI
  - `display_name`: Name displayed as the requester

#### **!! Important Note !!**
- The Spotify API does not permit retrieval of playlists owned by Spotify itself.
- If specified, the API returns a 404 Not Found error.
- Ensure that all configured playlists are public playlists created by Spotify users.
- The same applies to the BGM Provider Playlist.

### Playback Settings

- `notification_delay_ms`: Delay before playback start notifications in milliseconds (default: 3000)
  - Synchronizes notification timing with actual Spotify playback by delaying only the notification
  - Allows Spotify time to reflect playlist changes and buffers for network latency
  - Applies to EVERY track transition, ensuring the Spotify link in notifications is always ready
  - Recommended values: 2000-5000 milliseconds (2-5 seconds)
  - Set to 0 for no delay

- `gap_correction_ms`: Inter-track gap correction in milliseconds (default: 100)
  - Delays the start of the next track by this amount to compensate for Spotify client drift
  - Prevents server from getting ahead of the client over time
  - Applies to ALL track transitions
  - Recommended values: 50-200 milliseconds

### BGM Settings

- `depletion_threshold_sec`: Time before track ends to queue next track
- `recent_artist_count`: Number of recent artists to avoid duplicates
- `candidate_count`: Number of candidate tracks to fetch
- `providers`: Configured BGM providers (tried in order)
  - **Last.fm (experimental)**: Smart recommendations based on tags, similar tracks, and seeds
  - **Playlist**: Random selection from a Spotify playlist

### Filters

- `kicked_listener_filter`: Prevent kicked users from requesting
- `user_pending_filter`: Limit to one pending request per user
- `duplicate_track_filter`: Block duplicate tracks (including remasters)
- `duration_limit_filter`: Limit track duration (min/max minutes)


## Development

### Project Structure

```
├── cmd/             # Application entry points
├── internal/        # Internal packages
│   ├── api/         # API handlers
│   ├── app/         # Business logic
│   ├── gen/         # Generated Protocol Buffer code
│   └── infra/       # Infrastructure (Spotify, config, logging)
├── proto/           # Protocol Buffer definitions
└── config/          # Configuration files
```

### Generating Protocol Buffers

```bash
buf generate
```

### Testing

```bash
go test ./...
```

## Acknowledgments

- Built with [Connect](https://connectrpc.com/) for gRPC
- Spotify integration via [zmb3/spotify](https://github.com/zmb3/spotify)













