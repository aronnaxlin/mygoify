// Command poc checks the assumptions of milestone M0 against the real
// services: logging in to the Web API with PKCE (P3), and starting a
// playlist at one of its tracks on Soloist through the Web API (P4). It
// reads its settings from .env and keeps its token in the data directory.
//
//	go run ./cmd/poc login   logs in, in the browser
//	go run ./cmd/poc me      says who is logged in, refreshing the login
//	go run ./cmd/poc dump    prints Soloist's state and queue as it sends them
//	go run ./cmd/poc lib     reads the library through internal/spotify, changing nothing
//	go run ./cmd/poc p4      plays the third track of a playlist on Soloist
//	go run ./cmd/poc p6      plays a minute, measuring the position and Soloist's cost
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/coder/websocket"

	"mygoify/internal/auth"
	"mygoify/internal/music"
	"mygoify/internal/spotify"
)

var (
	env       map[string]string
	dataDir   = filepath.Join(home(), ".local/share/mygoify")
	tokenPath = filepath.Join(dataDir, "poc-token.json")
)

func main() {
	log.SetFlags(0)
	var err error
	if env, err = readEnv(".env"); err != nil {
		log.Fatal(err)
	}
	if len(os.Args) < 2 {
		log.Fatal("usage: poc login | me | dump | lib | p4 | p6")
	}
	switch os.Args[1] {
	case "login":
		err = login()
	case "me":
		var me struct {
			ID          string
			DisplayName string `json:"display_name"`
		}
		if err = api("GET", "/me", nil, &me); err == nil {
			fmt.Printf("logged in as %s (%s)\n", me.DisplayName, me.ID)
		}
	case "dump":
		err = dump()
	case "lib":
		err = readLibrary()
	case "p4":
		err = p4()
	case "p6":
		err = p6()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// readEnv reads KEY=value lines, expanding a leading ~ in values.
func readEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(v, "~/") {
			v = filepath.Join(home(), v[2:])
		}
		m[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return m, s.Err()
}

// fileStore keeps the token in a private file of the data directory.
type fileStore string

func (f fileStore) Load() ([]byte, error) { return os.ReadFile(string(f)) }
func (f fileStore) Save(b []byte) error {
	if err := os.MkdirAll(filepath.Dir(string(f)), 0o700); err != nil {
		return err
	}
	return os.WriteFile(string(f), b, 0o600)
}
func (f fileStore) Delete() error { return os.Remove(string(f)) }

func session() *auth.Session {
	return auth.NewSession(auth.Config{ClientID: env["SPOTIFY_CLIENT_ID"], RedirectURI: env["SPOTIFY_REDIRECT_URI"]}, fileStore(tokenPath))
}

// login logs in through internal/auth, in the browser, within 5 minutes.
func login() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	err := session().Login(ctx, func(u string) error {
		fmt.Println("Opening the browser to log in. If it doesn't open, visit:\n" + u)
		openBrowser(u)
		return nil
	})
	if err == nil {
		fmt.Println("Logged in.")
	}
	return err
}

func openBrowser(u string) {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	exec.Command(cmd, u).Start()
}

// accessToken returns a token good for a minute at least, refreshing the
// saved one if it isn't.
func accessToken() (string, error) {
	s := session()
	if !s.Restore() {
		return "", errors.New("not logged in: run poc login")
	}
	return s.AccessToken(context.Background())
}

// api calls the Web API, decoding the answer into out if it has one.
func api(method, path string, in, out any) error {
	tok, err := accessToken()
	if err != nil {
		return err
	}
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "https://api.spotify.com/v1"+path, body)
	req.Header.Set("Authorization", "Bearer "+tok)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s (Retry-After %q): %s", method, path, res.Status, res.Header.Get("Retry-After"), b)
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}

// soloistEvent is what P4 reads of Soloist's events.
type soloistEvent struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Item   struct {
		URI string `json:"uri"`
	} `json:"item"`
	Context struct {
		URI string `json:"uri"`
	} `json:"context"`
}

// p4 starts the third track of one of the listener's playlists on Soloist
// through the Web API, and watches Soloist's WebSocket for it to play.
func p4() error {
	var me struct{ ID string }
	if err := api("GET", "/me", nil, &me); err != nil {
		return err
	}

	var devices struct {
		Devices []struct {
			ID, Name, Type string
			IsActive       bool `json:"is_active"`
		}
	}
	if err := api("GET", "/me/player/devices", nil, &devices); err != nil {
		return err
	}
	deviceID := ""
	for _, d := range devices.Devices {
		fmt.Printf("device: %q %s active=%v\n", d.Name, d.Type, d.IsActive)
		if d.Name == env["SOLOIST_DEVICE_NAME"] {
			deviceID = d.ID
		}
	}
	if deviceID == "" {
		return fmt.Errorf("Soloist (%q) is not among the account's devices", env["SOLOIST_DEVICE_NAME"])
	}

	// A playlist of the listener's own with three tracks at least, since
	// Development Mode lists the items of those alone.
	var playlists struct {
		Items []struct {
			ID, Name, URI string
			Owner         struct{ ID string }
			Items         struct{ Total int } `json:"items"`
		}
	}
	if err := api("GET", "/me/playlists?limit=50", nil, &playlists); err != nil {
		return err
	}
	var pl struct{ ID, Name, URI string }
	for _, p := range playlists.Items {
		if p.Owner.ID == me.ID && p.Items.Total >= 3 {
			pl.ID, pl.Name, pl.URI = p.ID, p.Name, p.URI
			break
		}
	}
	if pl.ID == "" {
		return errors.New("no playlist of your own with three tracks or more")
	}
	var items struct {
		Items []struct {
			Item struct{ URI, Name string } `json:"item"`
		}
	}
	if err := api("GET", "/playlists/"+pl.ID+"/items?limit=3&offset=0", nil, &items); err != nil {
		return err
	}
	if len(items.Items) < 3 {
		return fmt.Errorf("playlist %q returned %d items", pl.Name, len(items.Items))
	}
	want := items.Items[2].Item
	fmt.Printf("playlist %q, its third track: %q %s\n", pl.Name, want.Name, want.URI)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ws, err := dialSoloist(ctx)
	if err != nil {
		return err
	}
	defer ws.CloseNow()

	start := time.Now()
	if err := api("PUT", "/me/player/play?device_id="+deviceID, map[string]any{
		"context_uri": pl.URI,
		"offset":      map[string]any{"position": 2},
	}, nil); err != nil {
		return err
	}
	fmt.Printf("Web API accepted the play request in %v\n", time.Since(start).Round(time.Millisecond))

	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return fmt.Errorf("waiting for Soloist to play it: %w", err)
		}
		var ev soloistEvent
		json.Unmarshal(data, &ev)
		if ev.Type == "playback_state" && ev.Status == "playing" && ev.Item.URI == want.URI {
			fmt.Printf("Soloist plays it, context %s, %v after the request\n", ev.Context.URI, time.Since(start).Round(time.Millisecond))
			break
		}
	}

	time.Sleep(5 * time.Second)
	pause, _ := json.Marshal(map[string]string{"type": "command", "command": "pause"})
	if err := ws.Write(ctx, websocket.MessageText, pause); err != nil {
		return err
	}
	fmt.Println("Paused after 5 seconds, through the WebSocket. P4 passed.")
	return nil
}

// dump prints what Soloist answers to get_state and get_queue, as it
// sends it, for writing the backend that reads it.
func dump() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, err := dialSoloist(ctx)
	if err != nil {
		return err
	}
	defer ws.CloseNow()
	for _, cmd := range []string{"get_auth_state", "get_state", "get_queue"} {
		if err := command(ctx, ws, cmd); err != nil {
			return err
		}
	}
	for seen := 0; seen < 3; {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return err
		}
		var ev struct{ Type string }
		json.Unmarshal(data, &ev)
		switch ev.Type {
		case "auth_state", "playback_state", "queue_changed", "error":
			fmt.Println(string(data))
			seen++
		}
	}
	return nil
}

// readLibrary reads through internal/spotify what the app shows, and says
// how much it found, changing nothing.
func readLibrary() error {
	s := session()
	if !s.Restore() {
		return errors.New("not logged in: run poc login")
	}
	l := &spotify.Library{API: &spotify.Client{Token: s.AccessToken}}
	step := func(name string, f func() (string, error)) {
		start := time.Now()
		out, err := f()
		if err != nil {
			out = "ERROR " + err.Error()
		}
		fmt.Printf("%-14s %6v  %s\n", name, time.Since(start).Round(time.Millisecond), out)
	}
	first := func(ts []music.Track) string {
		if len(ts) == 0 {
			return "-"
		}
		return fmt.Sprintf("%q by %s, cover %v", ts[0].Name, ts[0].ArtistNames(), len(ts[0].Album.Images) > 0)
	}
	var made, saved []music.Playlist
	var album, artist string
	step("Me", func() (string, error) {
		me, err := l.Me()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("id %s, %d images", me.ID, len(me.Images)), nil
	})
	step("Home", func() (string, error) {
		h, err := l.Home()
		if len(h.Recent) > 0 {
			album = h.Recent[0].Album.ID
			artist = h.Recent[0].Artists[0].ID
		}
		return fmt.Sprintf("%d recent, %d top tracks, %d top artists; recent: %s", len(h.Recent), len(h.TopTracks), len(h.TopArtists), first(h.Recent)), err
	})
	step("MyPlaylists", func() (string, error) {
		var err error
		made, saved, err = l.MyPlaylists()
		return fmt.Sprintf("%d made (first the liked songs: %d), %d saved", len(made), made[0].TrackCount, len(saved)), err
	})
	step("Playlist liked", func() (string, error) {
		p, err := l.Playlist("liked")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d tracks; %s", len(p.Tracks), first(p.Tracks)), nil
	})
	if len(made) > 1 {
		step("Playlist own", func() (string, error) {
			p, err := l.Playlist(made[1].ID)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d of %d tracks, hidden %v; %s", len(p.Tracks), p.TrackCount, p.Hidden, first(p.Tracks)), nil
		})
	}
	if len(saved) > 0 {
		step("Playlist saved", func() (string, error) {
			p, err := l.Playlist(saved[0].ID)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d tracks, hidden %v (by %s)", len(p.Tracks), p.Hidden, p.Owner.Name), nil
		})
	}
	step("LikedIDs", func() (string, error) {
		ids, err := l.LikedIDs()
		return fmt.Sprintf("%d", len(ids)), err
	})
	if album != "" {
		step("Album", func() (string, error) {
			a, err := l.Album(album)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%q, %s, %d songs, released %s", a.Name, a.ArtistNames(), len(a.Songs), a.Released), nil
		})
		step("Artist", func() (string, error) {
			a, err := l.Artist(artist)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%q, genres %v, %d songs, followed %v; %s", a.Name, a.Genres, len(a.Songs), a.Followed, first(a.Songs)), nil
		})
		step("ArtistAlbums", func() (string, error) {
			as, more, err := l.ArtistAlbums(artist, 30, 0)
			return fmt.Sprintf("%d, more %v", len(as), more), err
		})
	}
	for _, k := range []music.SearchType{music.SearchTracks, music.SearchArtists, music.SearchAlbums, music.SearchPlaylists} {
		step(fmt.Sprint("Search ", k), func() (string, error) {
			r, err := l.Search("春日影", k, 10, 0)
			return fmt.Sprintf("%d, more %v", r.Len(), r.More), err
		})
	}
	return nil
}
