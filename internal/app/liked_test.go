package app

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
	"mygoify/internal/playback"
)

// likedTracks are n songs, t0 to t(n-1), as the app shows them.
func likedTracks(n int) []music.Track {
	tracks := make([]music.Track, n)
	for i := range tracks {
		tracks[i] = music.Track{ID: fmt.Sprint("t", i), Name: fmt.Sprint("歌", i), Duration: 200000}
	}
	return tracks
}

// likedRig is a test app whose background answers land in posted, to run by
// hand, and whose pages are asked for.
type likedRig struct {
	t      *testing.T
	a      *app
	posted chan func()
	pages  chan struct{} // the app asking for the next page of liked songs
}

func newLikedRig(t *testing.T) *likedRig {
	r := &likedRig{t: t, a: newTestApp(), posted: make(chan func(), 8), pages: make(chan struct{}, 4)}
	r.a.settings = defaultSettings()
	r.a.profile = &music.User{ID: "1", Name: "小明"}
	r.a.inline = func(f func()) { r.posted <- f }
	return r
}

// run runs what the app posted next.
func (r *likedRig) run() {
	r.t.Helper()
	select {
	case f := <-r.posted:
		f()
	case <-time.After(5 * time.Second):
		r.t.Fatal("nothing posted")
	}
}

// noPage reports that the app asked for no page, once frames had a chance.
func (r *likedRig) noPage() {
	r.t.Helper()
	select {
	case <-r.pages:
		r.t.Fatal("the next page was asked for before the list scrolled")
	default:
	}
}

// The liked songs show a page at a time: the next one is asked for as the
// list comes to its foot, with rows of the skeleton in their place while it
// loads, and it shows once it does.
func TestLikedSongsPageAsTheyScroll(t *testing.T) {
	all := likedTracks(120)
	r := newLikedRig(t)
	a := r.a
	release := make(chan struct{})
	stub(a).playlist = func(string) (*music.Playlist, error) {
		return &music.Playlist{
			ID: "liked", Name: "喜欢的歌曲", Liked: true, Owner: music.User{ID: "1", Name: "小明"},
			Tracks: all[:50], TrackCount: len(all), More: true,
		}, nil
	}
	stub(a).likedMore = func() ([]music.Track, bool, error) {
		r.pages <- struct{}{}
		<-release
		return all[50:], false, nil
	}
	a.route = route{page: pagePlaylist, id: "liked"}
	a.loadPlaylist("liked", false)
	r.run()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if pl := a.playlists["liked"]; pl == nil || len(pl.Tracks) != 50 || !a.likedPage.more {
		t.Fatalf("the first page: %+v", pl)
	}
	r.noPage()

	tt.Scroll(800, 400, 0, 100000)
	tt.Advance(time.Second)
	tt.Frame()
	tt.Frame()
	select {
	case <-r.pages:
	case <-time.After(5 * time.Second):
		t.Fatal("the foot of the list asked for no page")
	}
	if !a.likedPage.loading {
		t.Error("the page after the first is not loading")
	}
	tt.Frame()
	close(release)
	r.run()
	tt.Frame()
	if pl := a.playlists["liked"]; len(pl.Tracks) != 120 || a.likedPage.more || a.likedPage.loading {
		t.Fatalf("after the page: %d songs, more %v, loading %v", len(pl.Tracks), a.likedPage.more, a.likedPage.loading)
	}
}

// The hearts ask which of the songs shown the listener likes, a batch for
// the songs of a page, and keep the answers: a song asked about once is not
// asked again as it shows again or the list scrolls on.
func TestHeartsAskAboutShownSongs(t *testing.T) {
	tracks := likedTracks(60)
	r := newLikedRig(t)
	a := r.a
	var mu sync.Mutex
	var asked [][]string
	stub(a).likes = func(ids []string) (map[string]bool, error) {
		mu.Lock()
		asked = append(asked, slices.Clone(ids))
		mu.Unlock()
		likes := make(map[string]bool, len(ids))
		for _, id := range ids {
			likes[id] = id == "t0"
		}
		return likes, nil
	}
	askedNow := func() [][]string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(asked)
	}
	askedCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(asked)
	}
	a.playlists["p1"] = &music.Playlist{ID: "p1", Name: "歌单", Tracks: tracks, TrackCount: len(tracks)}
	a.route = route{page: pagePlaylist, id: "p1"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	r.run()
	tt.Frame()
	if got := askedNow(); len(got) != 1 {
		t.Fatalf("asked %d times, of %d songs", len(got), len(tracks))
	} else if first := got[0]; !slices.Contains(first, "t0") || slices.Contains(first, "t59") || len(first) >= len(tracks) {
		t.Fatalf("asked about %v, of 60 songs", first)
	}
	if !a.isLiked(&tracks[0]) {
		t.Error("t0 is liked, and the app does not show it")
	}
	// Asked once, nothing more as the same songs show again.
	tt.Frame()
	tt.Frame()
	if got := askedNow(); len(got) != 1 {
		t.Fatalf("asked again: %v", got[1:])
	}
	// Scrolled, the songs that come are asked about, each once: frames
	// build the rows, and each batch's answer lands before the next.
	tt.Scroll(800, 400, 0, 1500)
	tt.Advance(time.Second)
	n := askedCount()
	for range 4 {
		tt.Frame()
		for askedCount() > n {
			r.run()
			n++
		}
	}
	got := askedNow()
	if len(got) == 1 {
		t.Fatal("the songs scrolled to were not asked about")
	}
	seen := map[string]bool{}
	for _, batch := range got {
		for _, id := range batch {
			if seen[id] {
				t.Errorf("%s was asked about twice", id)
			}
			seen[id] = true
		}
	}
	if len(seen) <= len(got[0]) {
		t.Errorf("scrolling asked nothing new: %d songs", len(seen))
	}
}

// A song liked while the answer about it was coming stays liked: what the
// listener did outranks the answer, which was asked for before.
func TestLikeWhileLikesAreAsked(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &music.User{ID: "1"}
	posted := make(chan func(), 2)
	a.inline = func(f func()) { posted <- f }
	release := make(chan struct{})
	stub(a).likes = func(ids []string) (map[string]bool, error) {
		<-release
		likes := make(map[string]bool, len(ids))
		for _, id := range ids {
			likes[id] = false // none is liked on Spotify
		}
		return likes, nil
	}
	tr := music.Track{ID: "x", Name: "歌"}
	a.playlists["p1"] = &music.Playlist{ID: "p1", Name: "歌单", Tracks: []music.Track{tr}, TrackCount: 1}
	a.route = route{page: pagePlaylist, id: "p1"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	a.setLiked(tr.ID, true)
	close(release)
	select {
	case f := <-posted:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("the answer about the songs was never posted")
	}
	if !a.isLiked(&tr) {
		t.Error("the stale answer unliked the song")
	}
	tt.Frame()
	tt.Frame()
	if !a.isLiked(&tr) {
		t.Error("a frame unliked the song")
	}
}

// playBackend plays as the fake does, but notes what it was asked to play.
type playBackend struct {
	*playback.Fake
	played chan playback.Request
}

func (b *playBackend) Play(req playback.Request) error {
	b.played <- req
	return b.Fake.Play(req)
}

// Playing the liked songs plays the listener's collection, from the song
// clicked: what plays on comes from Spotify's own list of those songs, not
// from the page loaded, which is only the first of them.
func TestLikedSongsPlayAsTheCollection(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.ClickPlays = true
	a.profile = &music.User{ID: "me", Name: "小明"}
	tracks := likedTracks(3)
	a.playlists["liked"] = &music.Playlist{
		ID: "liked", Name: "喜欢的歌曲", Liked: true, Owner: music.User{ID: "me", Name: "小明"},
		Tracks: tracks, TrackCount: 2000, More: true,
	}
	a.route = route{page: pagePlaylist, id: "liked"}
	played := make(chan playback.Request, 2)
	a.pl = playback.New(func(func()) {}, &playBackend{Fake: playback.NewFake(), played: played})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click(tracks[2].Name); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-played:
		if req.Context != "spotify:user:me:collection" || req.Start != 2 || len(req.Tracks) != len(tracks) {
			t.Fatalf("played %+v", req)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing played")
	}
}
