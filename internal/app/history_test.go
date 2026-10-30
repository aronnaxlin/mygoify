package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// The side buttons of a mouse go back and forward between pages.
func TestBackAndForward(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: []music.Track{{ID: "1", Name: "一张相片"}}}
	tt := ui.NewTester(a.view, 1200, 800)
	home, list := route{page: pageHome}, route{page: pagePlaylist, id: "42"}
	a.open(home)
	a.open(list)
	tt.Frame()

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route != home {
		t.Fatalf("back went to %+v", a.route)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != list {
		t.Fatalf("forward went to %+v", a.route)
	}

	// Back closes Now Playing before it leaves the page under it.
	a.pl.Cue([]music.Track{{ID: "1", Name: "一张相片"}})
	a.nowPlaying = true
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.nowPlaying || a.route != list {
		t.Fatalf("back with Now Playing open: open %v, page %+v", a.nowPlaying, a.route)
	}

	// A new page leaves nothing to go forward to.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	album := route{page: pageAlbum, id: "8"}
	a.open(album)
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != album {
		t.Fatalf("forward after a new page went to %+v", a.route)
	}
}
