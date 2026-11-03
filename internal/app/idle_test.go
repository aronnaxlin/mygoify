package app

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/library"
	"mygoify/internal/music"
)

// countingLib tells of each call made to the library, which with Spotify
// is a call to the Web API.
type countingLib struct {
	library.Library
	mu    sync.Mutex
	calls []string
	likes [][]string // the songs each call to Likes asked about
}

func (l *countingLib) called(name string) {
	l.mu.Lock()
	l.calls = append(l.calls, name)
	l.mu.Unlock()
}

// take returns what was asked since the last take: its calls, and the
// songs each of them asked about.
func (l *countingLib) take() (calls []string, likes [][]string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	calls, likes = l.calls, l.likes
	l.calls, l.likes = nil, nil
	return calls, likes
}

func (l *countingLib) Me() (*music.User, error)  { l.called("Me"); return l.Library.Me() }
func (l *countingLib) Home() (music.Home, error) { l.called("Home"); return l.Library.Home() }
func (l *countingLib) MyPlaylists() ([]music.Playlist, []music.Playlist, error) {
	l.called("MyPlaylists")
	return l.Library.MyPlaylists()
}
func (l *countingLib) Playlist(id string) (*music.Playlist, error) {
	l.called("Playlist")
	return l.Library.Playlist(id)
}
func (l *countingLib) Album(id string) (*music.AlbumProfile, error) {
	l.called("Album")
	return l.Library.Album(id)
}
func (l *countingLib) Artist(id string) (*music.ArtistProfile, error) {
	l.called("Artist")
	return l.Library.Artist(id)
}
func (l *countingLib) ArtistAlbums(id string, limit, offset int) ([]music.AlbumProfile, bool, error) {
	l.called("ArtistAlbums")
	return l.Library.ArtistAlbums(id, limit, offset)
}
func (l *countingLib) Search(q string, kind music.SearchType, limit, offset int) (music.SearchResult, error) {
	l.called("Search")
	return l.Library.Search(q, kind, limit, offset)
}
func (l *countingLib) Likes(ids []string) (map[string]bool, error) {
	l.mu.Lock()
	l.calls = append(l.calls, "Likes")
	l.likes = append(l.likes, slices.Clone(ids))
	l.mu.Unlock()
	return l.Library.Likes(ids)
}
func (l *countingLib) LikedMore() ([]music.Track, bool, error) {
	l.called("LikedMore")
	return l.Library.LikedMore()
}
func (l *countingLib) Like(id string, like bool) error {
	l.called("Like")
	return l.Library.Like(id, like)
}
func (l *countingLib) FollowArtist(id string, follow bool) error {
	l.called("FollowArtist")
	return l.Library.FollowArtist(id, follow)
}
func (l *countingLib) CreatePlaylist(name string) (*music.Playlist, error) {
	l.called("CreatePlaylist")
	return l.Library.CreatePlaylist(name)
}
func (l *countingLib) RenamePlaylist(id, name string) error {
	l.called("RenamePlaylist")
	return l.Library.RenamePlaylist(id, name)
}
func (l *countingLib) AddToPlaylist(id string, ids ...string) error {
	l.called("AddToPlaylist")
	return l.Library.AddToPlaylist(id, ids...)
}
func (l *countingLib) RemoveFromPlaylist(id string, ids ...string) error {
	l.called("RemoveFromPlaylist")
	return l.Library.RemoveFromPlaylist(id, ids...)
}
func (l *countingLib) DeletePlaylist(id string) error {
	l.called("DeletePlaylist")
	return l.Library.DeletePlaylist(id)
}

// Left alone with the window hidden, the app asks Spotify nothing however
// long the music plays, even while its view is built: the developer
// account's calls are few (DESIGN.md §5.3), and a refresh on a timer would
// spend them all day in the menu bar. Shown, the window asks about the song
// playing, so that the player bar's heart is right: once for each song
// whose heart it does not know, and not again for one it does.
func TestIdleAsksNothing(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	f := library.NewFake()
	// What boot would have loaded.
	a.profile, _ = f.Me()
	a.home, _ = f.Home()
	a.homeAt = time.Now()
	a.mine, a.saved, _ = f.MyPlaylists()
	lib := &countingLib{Library: a.lib}
	a.lib = lib
	// The answers come on a goroutine, as with Spotify: run them by hand.
	answered := make(chan func(), 64)
	a.inline = func(fn func()) { answered <- fn }
	drain := func() {
		for {
			select {
			case fn := <-answered:
				fn()
			default:
				return
			}
		}
	}
	shows := false
	original := windowShows
	windowShows = func(*app) bool { return shows }
	t.Cleanup(func() { windowShows = original })
	n := &notch{a: a, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 240}
	a.notch = n
	a.settings.Notch = true
	tracks := []music.Track{{ID: "1", Name: "一", Duration: 200000}, {ID: "2", Name: "二", Duration: 200000}, {ID: "3", Name: "三", Duration: 200000}}
	a.pl.Cue(tracks)
	a.pl.Start()

	// The window's view is built either way, as it is when it is shown or
	// hidden; the notch's always.
	views := []*ui.Tester{ui.NewTester(n.view, int(n.winW), int(n.winH)), ui.NewTester(a.view, 1200, 800)}
	play := func(steps int) {
		for step := range steps {
			for _, tt := range views {
				tt.Advance(time.Minute)
				tt.Frame()
			}
			// A song ends every few minutes.
			if step%4 == 3 {
				a.pl.Next()
				a.syncWindows()
			}
			drain()
		}
	}

	play(60)
	// What the app asks, it asks on a goroutine.
	time.Sleep(50 * time.Millisecond)
	drain()
	if calls, _ := lib.take(); len(calls) > 0 {
		t.Fatalf("asked Spotify %v with the window hidden", calls)
	}

	shows = true
	play(60)
	time.Sleep(50 * time.Millisecond)
	drain()
	calls, likes := lib.take()
	asked := map[string]int{}
	for _, batch := range likes {
		for _, id := range batch {
			asked[id]++
		}
	}
	for _, call := range calls {
		if call != "Likes" {
			t.Errorf("asked the library %s while the listener did nothing", call)
		}
	}
	for i := range tracks {
		if n := asked[tracks[i].ID]; n != 1 {
			t.Errorf("asked about %s %d times while it played, want once", tracks[i].Name, n)
		}
	}
	if len(asked) != len(tracks) {
		t.Errorf("asked about %v, of the songs that played", asked)
	}
}
