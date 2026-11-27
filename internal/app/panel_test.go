package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
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

// Escape closes the mini player.
func TestMiniClosesOnEscape(t *testing.T) {
	_, tt, _, closed := miniApp(t)
	tt.Key(0, ui.KeyEscape)
	if !*closed {
		t.Error("still open")
	}
}
