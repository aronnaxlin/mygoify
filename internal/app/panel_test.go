package app

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
	"mygoify/internal/playback"
)

// miniApp is the mini player playing the first of three songs of an
// album, as it shows off Omarchy, with what it opens and whether it closed.
func miniApp(t *testing.T) (*app, *ui.Tester, *[]string, *bool) {
	t.Setenv("MYGOIFY_OMARCHY", "0")
	a := newTestApp()
	a.settings = defaultSettings()
	a.mini = true
	al := music.Album{ID: "8", Name: "迷跡波"}
	band := []music.Artist{{ID: "7", Name: "MyGO!!!!!"}}
	a.pl.Cue([]music.Track{
		{ID: "1", Name: "春日影", Artists: band, Album: al, Duration: 200000},
		{ID: "2", Name: "迷星叫", Artists: band, Album: al, Duration: 200000},
		{ID: "3", Name: "碧天伴走", Artists: band, Album: al, Duration: 200000},
	})
	var opened []string
	closed := false
	old, oldQuit := openApp, quitApp
	openApp = func(args ...string) { opened = append(opened, args...) }
	quitApp = func() { closed = true }
	t.Cleanup(func() { openApp, quitApp = old, oldQuit })
	tt := ui.NewTester(a.miniView, miniWidth, miniHeight)
	tt.Frame()
	return a, tt, &opened, &closed
}

// A song of what plays next, clicked, plays.
func TestMiniPlaysFromQueue(t *testing.T) {
	a, tt, _, _ := miniApp(t)
	if err := tt.Click("碧天伴走"); err != nil {
		t.Fatal(err)
	}
	if id := a.pl.Current().ID; id != "3" {
		t.Errorf("playing %s", id)
	}
}

// What plays, clicked, opens its album in the app, and the mini player
// closes: it has no page of its own to open.
func TestMiniOpensAlbumInApp(t *testing.T) {
	a, tt, opened, closed := miniApp(t)
	// Its name, at the start of the line it fills.
	r, ok := tt.Find("春日影")
	if !ok {
		t.Fatal("no name")
	}
	tt.ClickAt(r.X+8, r.Y+r.H/2)
	if !slices.Equal(*opened, []string{"spotify:album:8"}) || !*closed {
		t.Errorf("opened %q, closed %v", *opened, *closed)
	}
	if a.route.page != pageHome {
		t.Errorf("opened a page of its own: %+v", a.route)
	}
}

// Its album's name opens the album in the app. It showed as a caption,
// and opened nothing, as its artists did.
func TestMiniOpensAlbumFromItsName(t *testing.T) {
	_, tt, opened, closed := miniApp(t)
	r, ok := tt.Find("迷跡波")
	if !ok {
		t.Fatal("no album")
	}
	tt.ClickAt(r.X+4, r.Y+r.H/2)
	if !slices.Equal(*opened, []string{"spotify:album:8"}) || !*closed {
		t.Errorf("opened %q, closed %v", *opened, *closed)
	}
}

// A song up next, its play-next button clicked as the pointer is over it,
// is queued to play after this one, and doesn't play at once.
func TestMiniPlaysNext(t *testing.T) {
	t.Setenv("MYGOIFY_OMARCHY", "0")
	a := newTestApp()
	a.settings = defaultSettings()
	a.mini = true
	b := &queueBackend{Fake: playback.NewFake()}
	posted := make(chan func(), 8)
	a.pl = playback.New(func(f func()) { posted <- f }, b)
	a.pl.Cue([]music.Track{{ID: "1", Name: "春日影", Duration: 200000}, {ID: "2", Name: "迷星叫", Duration: 200000}, {ID: "3", Name: "碧天伴走", Duration: 200000}})
	tt := ui.NewTester(a.miniView, miniWidth, miniHeight)
	tt.Frame()
	r, ok := tt.Find("碧天伴走")
	if !ok {
		t.Fatal("no song up next")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if err := tt.Click("下一首播放"); err != nil {
		t.Fatal(err)
	}
	select {
	case f := <-posted:
		f()
	case <-time.After(2 * time.Second):
		t.Fatal("the backend was never asked")
	}
	if !slices.Equal(b.queued, []string{"3"}) || a.pl.Current().ID != "1" {
		t.Errorf("queued %v, playing %s", b.queued, a.pl.Current().ID)
	}
}

// Escape closes the mini player.
func TestMiniClosesOnEscape(t *testing.T) {
	_, tt, _, closed := miniApp(t)
	tt.Key(0, ui.KeyEscape)
	if !*closed {
		t.Error("still open")
	}
}
