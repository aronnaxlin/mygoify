// Command poc checks the assumptions of milestone M0 against the real
// services: logging in to the Web API with PKCE (P3), and starting a
// playlist at one of its tracks on Soloist through the Web API (P4). It
// reads its settings from .env and keeps its token in the data directory.
//
//	go run ./cmd/poc login   logs in, in the browser
//	go run ./cmd/poc p4      plays the third track of a playlist on Soloist
//	go run ./cmd/poc p6      plays a minute, measuring the position and Soloist's cost
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// The scopes of DESIGN.md §5.1: only what the app uses.
const scopes = "user-read-playback-state user-modify-playback-state user-read-currently-playing " +
	"user-read-recently-played user-top-read user-library-read user-library-modify " +
	"playlist-read-private playlist-read-collaborative playlist-modify-private playlist-modify-public"

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
		log.Fatal("usage: poc login | p4 | p6")
	}
	switch os.Args[1] {
	case "login":
		err = login()
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

type token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	Scope        string    `json:"scope"`
	Expiry       time.Time `json:"expiry"`
}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// login runs the authorization code flow with PKCE: the browser asks the
// listener, Spotify sends the code to the loopback address, and the code
// and the verifier buy the token.
func login() error {
	redirect, err := url.Parse(env["SPOTIFY_REDIRECT_URI"])
	if err != nil {
		return err
	}
	verifier := randomString(64)
	sum := sha256.Sum256([]byte(verifier))
	state := randomString(16)
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {env["SPOTIFY_CLIENT_ID"]},
		"redirect_uri":          {redirect.String()},
		"scope":                 {scopes},
		"state":                 {state},
		"code_challenge_method": {"S256"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
	}
	authURL := "https://accounts.spotify.com/authorize?" + q.Encode()

	ln, err := net.Listen("tcp4", redirect.Host)
	if err != nil {
		return err
	}
	codes := make(chan string, 1)
	errs := make(chan error, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != redirect.Path {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		switch {
		case q.Get("state") != state:
			errs <- errors.New("the callback's state does not match")
		case q.Get("error") != "":
			errs <- fmt.Errorf("authorization refused: %s", q.Get("error"))
		default:
			fmt.Fprintln(w, "MyGoify: logged in. You can close this tab.")
			codes <- q.Get("code")
			return
		}
		fmt.Fprintln(w, "MyGoify: login failed, see the terminal.")
	})}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Println("Opening the browser to log in. If it doesn't open, visit:\n" + authURL)
	openBrowser(authURL)

	var code string
	select {
	case code = <-codes:
	case err := <-errs:
		return err
	case <-time.After(5 * time.Minute):
		return errors.New("no login within 5 minutes")
	}
	tok, err := requestToken(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirect.String()},
		"client_id":     {env["SPOTIFY_CLIENT_ID"]},
		"code_verifier": {verifier},
	})
	if err != nil {
		return err
	}
	fmt.Printf("Logged in. Granted scopes: %s\n", tok.Scope)
	return nil
}

func openBrowser(u string) {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	exec.Command(cmd, u).Start()
}

// requestToken posts to the token endpoint and saves what it returns,
// keeping the refresh token when a refresh returns none.
func requestToken(form url.Values) (*token, error) {
	res, err := http.PostForm("https://accounts.spotify.com/api/token", form)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint: %s: %s", res.Status, body)
	}
	var tok token
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = form.Get("refresh_token")
	}
	tok.Expiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	data, _ := json.MarshalIndent(tok, "", "  ")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return &tok, os.WriteFile(tokenPath, data, 0o600)
}

// accessToken returns a token valid for a minute at least, refreshing the
// saved one if it isn't.
func accessToken() (string, error) {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("not logged in: run poc login (%w)", err)
	}
	var tok token
	if err := json.Unmarshal(data, &tok); err != nil {
		return "", err
	}
	if time.Until(tok.Expiry) > time.Minute {
		return tok.AccessToken, nil
	}
	t, err := requestToken(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
		"client_id":     {env["SPOTIFY_CLIENT_ID"]},
	})
	if err != nil {
		return "", err
	}
	return t.AccessToken, nil
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
