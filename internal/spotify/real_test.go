package spotify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"mygoify/internal/auth"
	"mygoify/internal/music"
	"mygoify/internal/secret"
)

// counting records the calls that go to the Web API, without their
// headers: the token stays out of the report.
type counting struct {
	mu    sync.Mutex
	calls []call
}

type call struct {
	path   string
	status int
	took   time.Duration
}

func (c *counting) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	res, err := http.DefaultTransport.RoundTrip(req)
	status := 0
	if res != nil {
		status = res.StatusCode
	}
	c.mu.Lock()
	c.calls = append(c.calls, call{req.URL.Path, status, time.Since(start)})
	c.mu.Unlock()
	return res, err
}

func (c *counting) take() []call {
	c.mu.Lock()
	defer c.mu.Unlock()
	calls := c.calls
	c.calls = nil
	return calls
}

func report(t *testing.T, step string, calls []call) {
	byPath := map[string]int{}
	for _, c := range calls {
		if c.status == http.StatusTooManyRequests {
			t.Errorf("%s: %s rate limited", step, c.path)
		}
		byPath[strings.TrimPrefix(c.path, "/v1")]++
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	t.Logf("%s: %d calls", step, len(calls))
	for _, p := range paths {
		t.Logf("    %3d  %s", byPath[p], p)
	}
}

// realLibrary signs in as the app does, with the login it keeps and the
// client ID of the settings of the dev build, or SPOTIFY_CLIENT_ID.
func realLibrary(t *testing.T, cache string, hc *http.Client) *Library {
	id := os.Getenv("SPOTIFY_CLIENT_ID")
	if id == "" {
		home, _ := os.UserHomeDir()
		var s struct {
			Spotify struct {
				ClientID string `json:"clientId"`
			} `json:"spotify"`
		}
		b, _ := os.ReadFile(filepath.Join(home, "Library/Application Support/MyGoify Dev/settings.json"))
		json.Unmarshal(b, &s)
		id = s.Spotify.ClientID
	}
	if id == "" {
		t.Fatal("no client ID: set SPOTIFY_CLIENT_ID")
	}
	home, _ := os.UserHomeDir()
	keep := secret.Default(filepath.Join(home, ".local/share/mygoify"))
	sess := auth.NewSession(auth.Config{ClientID: id}, secret.Entry{Store: keep, Name: secret.SpotifyLogin})
	if !sess.Restore() {
		t.Fatal("no Spotify login kept: log in through the app first")
	}
	return &Library{API: &Client{Token: sess.AccessToken, HTTP: hc}, Cache: cache}
}

// TestRequestsReal counts the calls to the Web API that browsing the
// library makes, from an empty cache and again from the cache kept, on
// the listener's real account. MYGOIFY_REAL=1 runs it.
func TestRequestsReal(t *testing.T) {
	if os.Getenv("MYGOIFY_REAL") == "" {
		t.Skip("MYGOIFY_REAL not set")
	}
	tr := &counting{}
	h := &http.Client{Transport: tr}
	cache := t.TempDir()

	browse := func(l *Library) {
		must := func(err error) {
			if err != nil {
				t.Fatal(err)
			}
		}
		_, err := l.Me()
		must(err)
		home, err := l.Home()
		must(err)
		made, saved, err := l.MyPlaylists()
		must(err)
		for _, p := range append(made[:min(3, len(made))], saved[:min(2, len(saved))]...) {
			_, err := l.Playlist(p.ID)
			must(err)
		}
		_, err = l.LikedIDs()
		must(err)
		if len(home.Recent) > 0 {
			_, err := l.Album(home.Recent[0].Album.ID)
			must(err)
		}
		if len(home.TopArtists) > 0 {
			_, err := l.Artist(home.TopArtists[0].ID)
			must(err)
		}
	}

	start := time.Now()
	browse(realLibrary(t, cache, h))
	report(t, fmt.Sprintf("cold start (%v)", time.Since(start).Round(time.Millisecond)), tr.take())

	// As the app started again.
	start = time.Now()
	browse(realLibrary(t, cache, h))
	report(t, fmt.Sprintf("warm start (%v)", time.Since(start).Round(time.Millisecond)), tr.take())
}

// TestSearchReal searches the listener's real account as the app does,
// and pages on as "显示更多" does, from the count of results shown: each
// first page should come within a second, and no result show twice.
// MYGOIFY_REAL=1 runs it.
func TestSearchReal(t *testing.T) {
	if os.Getenv("MYGOIFY_REAL") == "" {
		t.Skip("MYGOIFY_REAL not set")
	}
	tr := &counting{}
	l := realLibrary(t, "", &http.Client{Transport: tr})
	kinds := []struct {
		name string
		kind music.SearchType
	}{{"tracks", music.SearchTracks}, {"artists", music.SearchArtists}, {"albums", music.SearchAlbums}, {"playlists", music.SearchPlaylists}}
	const pages = 3
	for _, q := range []string{"MyGO!!!!!", "周杰伦", "radiohead"} {
		for _, k := range kinds {
			seen := map[string]int{}
			shown, skipped, dups := 0, 0, 0
			var first time.Duration
			for page := 0; page < pages; page++ {
				start := time.Now()
				res, err := l.Search(q, k.kind, 10, shown+skipped)
				took := time.Since(start)
				if err != nil {
					t.Fatalf("%s %s page %d: %v", q, k.name, page, err)
				}
				if page == 0 {
					first = took
				}
				for _, id := range ids(res) {
					if seen[id]++; seen[id] == 2 {
						dups++
					}
				}
				shown += res.Len()
				skipped += res.Skipped
				if res.Len() == 0 || !res.More {
					break
				}
			}
			t.Logf("%-10s %-9s first page %4dms, %2d shown, %d skipped, %d twice", q, k.name, first.Milliseconds(), shown, skipped, dups)
			if first > time.Second {
				t.Errorf("%s %s: first page took %v", q, k.name, first)
			}
			if dups > 0 {
				t.Errorf("%s %s: %d results shown twice", q, k.name, dups)
			}
		}
	}
	report(t, "search", tr.take())
}

func ids(r music.SearchResult) []string {
	var out []string
	for _, x := range r.Tracks {
		out = append(out, x.ID)
	}
	for _, x := range r.Artists {
		out = append(out, x.ID)
	}
	for _, x := range r.Albums {
		out = append(out, x.ID)
	}
	for _, x := range r.Playlists {
		out = append(out, x.ID)
	}
	return out
}
