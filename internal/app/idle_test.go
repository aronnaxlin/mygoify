package app

import (
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
}

func (l *countingLib) called(name string) {
	l.mu.Lock()
	l.calls = append(l.calls, name)
	l.mu.Unlock()
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
func (l *countingLib) LikedIDs() ([]string, error) { l.called("LikedIDs"); return l.Library.LikedIDs() }
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

// Left alone once it loaded, the window hidden or shown, the app asks
// Spotify nothing however long the music plays: the developer account's
// calls are few (DESIGN.md §5.3), and a refresh on a timer would spend
// them all day in the menu bar.
func TestIdleAsksNothing(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	f := library.NewFake()
	// What boot would have loaded.
	a.profile, _ = f.Me()
	a.home, _ = f.Home()
	a.homeAt = time.Now()
	a.mine, a.saved, _ = f.MyPlaylists()
	a.liked = map[string]bool{}
	lib := &countingLib{Library: a.lib}
	a.lib = lib
	n := &notch{a: a, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 240}
	a.notch = n
	a.settings.Notch = true
	tracks := []music.Track{{ID: "1", Name: "一", Duration: 200000}, {ID: "2", Name: "二", Duration: 200000}, {ID: "3", Name: "三", Duration: 200000}}
	a.pl.Cue(tracks)
	a.pl.Start()

	// Hidden, only the notch is drawn; shown, the window is too.
	notchView := ui.NewTester(n.view, int(n.winW), int(n.winH))
	var mainView *ui.Tester
	for step := range 120 {
		if step == 60 {
			mainView = ui.NewTester(a.view, 1200, 800)
		}
		for _, tt := range []*ui.Tester{notchView, mainView} {
			if tt != nil {
				tt.Advance(time.Minute)
				tt.Frame()
			}
		}
		// A song ends every few minutes.
		if step%4 == 3 {
			a.pl.Next()
			a.syncWindows()
		}
	}
	// What the app asks, it asks on a goroutine.
	time.Sleep(50 * time.Millisecond)
	lib.mu.Lock()
	defer lib.mu.Unlock()
	if len(lib.calls) > 0 {
		t.Errorf("asked Spotify %v while left alone", lib.calls)
	}
}
