package app

import (
	"os"
	"path/filepath"

	"mygoify/internal/auth"
	"mygoify/internal/library"
	"mygoify/internal/playback"
	"mygoify/internal/soloist"
	"mygoify/internal/spotify"
)

// Until onboarding keeps what the app plays through and logs in with
// (M2), it takes it from the environment, as .env sets it in development:
//
//	set -a; . ./.env; set +a; go tool mygo dev
//
// SPOTIFY_CLIENT_ID and SPOTIFY_REDIRECT_URI log in to the Web API, with
// the token cmd/poc login kept; SOLOIST_DEVICE_NAME names the Soloist
// running with its data in SOLOIST_DATA_DIR, or ~/.local/share/mygoify/soloist.
// Without them, the app shows the fake library and plays through the
// fake backend.

// devSpotify returns the library and the backend the environment names,
// or nils without them.
func devSpotify() (library.Library, playback.Backend) {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	if clientID == "" {
		return nil, nil
	}
	home, _ := os.UserHomeDir()
	data := filepath.Join(home, ".local/share/mygoify")
	dir := os.Getenv("SOLOIST_DATA_DIR")
	if dir == "" {
		dir = filepath.Join(data, "soloist")
	}
	name := os.Getenv("SOLOIST_DEVICE_NAME")
	if name == "" {
		name = "MyGoify"
	}
	sess := auth.NewSession(auth.Config{ClientID: clientID, RedirectURI: os.Getenv("SPOTIFY_REDIRECT_URI")}, tokenFile(filepath.Join(data, "poc-token.json")))
	if !sess.Restore() {
		return nil, nil
	}
	api := &spotify.Client{Token: sess.AccessToken}
	return &spotify.Library{API: api}, soloist.New(soloist.Config{DataDir: dir, DeviceName: name, API: api})
}

// tokenFile keeps the token in a private file, as cmd/poc does.
type tokenFile string

func (f tokenFile) Load() ([]byte, error) { return os.ReadFile(string(f)) }
func (f tokenFile) Save(b []byte) error   { return os.WriteFile(string(f), b, 0o600) }
func (f tokenFile) Delete() error         { return os.Remove(string(f)) }
