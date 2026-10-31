package app

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"mygoify/internal/auth"
	"mygoify/internal/library"
	"mygoify/internal/playback"
	"mygoify/internal/secret"
	"mygoify/internal/soloist"
	"mygoify/internal/spotify"
)

// spotifySettings are what the app logs in with and plays through
// (DESIGN.md §4.2): onboarding fills them in, and the secrets go to the
// secret store. Each left empty is taken from the environment, as .env
// sets it in development:
//
//	set -a; . ./.env; set +a; go tool mygo dev
//
// as SPOTIFY_CLIENT_ID, SPOTIFY_REDIRECT_URI, SOLOIST_BIN and
// SOLOIST_DEVICE_NAME; SOLOIST_API_KEY stands for the key kept, and
// SOLOIST_DATA_DIR moves Soloist's data out of ~/.local/share/mygoify/soloist.
type spotifySettings struct {
	ClientID    string `json:"clientId,omitempty"`
	RedirectURI string `json:"redirectUri,omitempty"`
	// SoloistBin is Soloist's executable; without it, the app plays
	// through a Soloist started by hand.
	SoloistBin string `json:"soloistBin,omitempty"`
	// DeviceName is how Spotify Connect lists Soloist, "MyGoify" unless
	// the listener names it.
	DeviceName string `json:"deviceName,omitempty"`
}

// withEnv fills in what s leaves empty from the environment.
func (s spotifySettings) withEnv() spotifySettings {
	for _, f := range []struct {
		v   *string
		env string
	}{
		{&s.ClientID, "SPOTIFY_CLIENT_ID"},
		{&s.RedirectURI, "SPOTIFY_REDIRECT_URI"},
		{&s.SoloistBin, "SOLOIST_BIN"},
		{&s.DeviceName, "SOLOIST_DEVICE_NAME"},
	} {
		if *f.v == "" {
			*f.v = os.Getenv(f.env)
		}
	}
	if s.DeviceName == "" {
		s.DeviceName = "MyGoify"
	}
	return s
}

// dataDir is where the app keeps what is the listener's: Soloist's data,
// the files of the secret store.
func dataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local/share/mygoify")
}

// secrets is the store of the listener's secrets.
func secrets() secret.Store { return secret.Default(dataDir()) }

// connectSpotify returns the library and the backend of Spotify, as cfg
// and the secrets kept set them up, or nils with no login.
func connectSpotify(cfg spotifySettings, keep secret.Store) (library.Library, playback.Backend) {
	cfg = cfg.withEnv()
	if cfg.ClientID == "" {
		return nil, nil
	}
	data := dataDir()
	dir := os.Getenv("SOLOIST_DATA_DIR")
	if dir == "" {
		dir = filepath.Join(data, "soloist")
	}
	adoptPOCLogin(keep, filepath.Join(data, "poc-token.json"))
	sess := auth.NewSession(auth.Config{ClientID: cfg.ClientID, RedirectURI: cfg.RedirectURI}, secret.Entry{Store: keep, Name: secret.SpotifyLogin})
	if !sess.Restore() {
		return nil, nil
	}
	if bin, key := cfg.SoloistBin, soloistKey(keep); bin != "" && key != "" {
		home, _ := os.UserHomeDir()
		if rest, ok := strings.CutPrefix(bin, "~/"); ok {
			bin = filepath.Join(home, rest)
		}
		svc := &soloist.Service{Bin: bin, Key: key, DeviceName: cfg.DeviceName, DataDir: dir, Logs: filepath.Join(data, "soloist.log")}
		// The backend connects once it listens.
		go func() {
			if err := svc.Start(); err != nil {
				log.Print(err)
			}
		}()
	}
	api := &spotify.Client{Token: sess.AccessToken, HoldFile: filepath.Join(data, "spotify-hold")}
	lib := &spotify.Library{API: api}
	if c, err := os.UserCacheDir(); err == nil {
		lib.Cache = filepath.Join(c, "mygoify", "spotify")
	}
	return lib, soloist.New(soloist.Config{DataDir: dir, DeviceName: cfg.DeviceName, API: api, DeviceFile: filepath.Join(data, "soloist-device")})
}

// soloistKey is the API key kept, or else SOLOIST_API_KEY's.
func soloistKey(keep secret.Store) string {
	if b, err := keep.Get(secret.SoloistKey); err == nil && len(b) > 0 {
		return strings.TrimSpace(string(b))
	} else if err != nil && !errors.Is(err, secret.ErrNotFound) {
		log.Print(err)
	}
	return os.Getenv("SOLOIST_API_KEY")
}

// adoptPOCLogin moves the login cmd/poc kept in a file of its own into
// the store, unless the store keeps one already.
func adoptPOCLogin(keep secret.Store, file string) {
	if _, err := keep.Get(secret.SpotifyLogin); !errors.Is(err, secret.ErrNotFound) {
		return
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return
	}
	if err := keep.Set(secret.SpotifyLogin, b); err != nil {
		log.Print(err)
		return
	}
	os.Remove(file)
}
