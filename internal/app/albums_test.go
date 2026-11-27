package app

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// albumApp is an app on the page of the fake library's album 迷跡波, its
// answers run as the test runs what was posted.
func albumApp(t *testing.T) (*app, *ui.Tester, func()) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &music.User{ID: "1"}
	posted := make(chan func(), 16)
	a.inline = func(f func()) { posted <- f }
	res, err := a.lib.Search("迷跡波", music.SearchAlbums, 1, 0)
	if err != nil || len(res.Albums) == 0 {
		t.Fatal("no album 迷跡波")
	}
	// answered runs what the library answered, as it comes, and redraws.
	tt := ui.NewTester(a.view, 1200, 800)
	answered := func() {
		for {
			select {
			case f := <-posted:
				f()
			case <-time.After(100 * time.Millisecond):
				tt.Frame()
				return
			}
		}
	}
	a.open(route{page: pageAlbum, id: res.Albums[0].ID})
	answered()
	return a, tt, answered
}

// 收藏 saves an album at once, and asks Spotify to; a second click waits
// for the first, and a save Spotify refuses goes back.
func TestSaveAlbum(t *testing.T) {
	a, tt, answered := albumApp(t)
	asked := make(chan bool, 4)
	refuse := false
	stub(a).saveAlbum = func(id string, save bool) error {
		asked <- save
		if refuse {
			return errors.New("offline")
		}
		return nil
	}
	if err := tt.Click("收藏"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !wait(t, asked, "the save") || !a.albums[a.route.id].album.Saved {
		t.Fatal("not saved")
	}
	tt.Click("已收藏")
	if len(asked) > 0 || !a.albums[a.route.id].album.Saved {
		t.Error("unsaved before the save landed")
	}
	answered()
	refuse = true
	tt.Click("已收藏")
	tt.Frame()
	if wait(t, asked, "the unsave") {
		t.Fatal("asked to save again")
	}
	answered()
	if !a.albums[a.route.id].album.Saved {
		t.Error("a refused unsave stayed")
	}
}

// An album saved shows on the page of saved albums, and one let go leaves
// it, as the page opens again.
func TestSavedAlbumsPage(t *testing.T) {
	a, tt, answered := albumApp(t)
	album := a.route
	tt.Click("收藏")
	answered()
	if err := tt.Click("收藏的专辑"); err != nil {
		t.Fatal(err)
	}
	answered()
	if !tt.HasText("迷跡波") {
		t.Fatal("the album saved isn't listed")
	}
	tt.Click("迷跡波")
	tt.Frame()
	if a.route != album {
		t.Fatalf("opened %+v", a.route)
	}
	tt.Click("已收藏")
	answered()
	a.back()
	answered()
	if tt.HasText("迷跡波") {
		t.Error("the album let go is still listed")
	}
}
