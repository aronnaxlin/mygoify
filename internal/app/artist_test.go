package app

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// Each artist's name opens their page: from Now Playing, which closes, and
// from a song's row, which doesn't play the song as the row's click would.
func TestArtistNameOpensTheirPage(t *testing.T) {
	a := newTestApp()
	stub(a).artist = func(string) (*music.ArtistProfile, error) { return nil, errors.New("offline") }
	a.settings = defaultSettings()
	duet := music.Track{ID: "1", Name: "广寒宫", Artists: []music.Artist{{ID: "11", Name: "甲"}, {ID: "12", Name: "乙"}}, Duration: 200000}
	a.pl.Cue([]music.Track{duet})
	a.setLyrics(nil)
	a.artists["12"] = &artistData{profile: &music.ArtistProfile{
		Artist: music.Artist{ID: "12", Name: "乙"},
		Songs:  []music.Track{{ID: "2", Name: "起风了", Artists: []music.Artist{{ID: "13", Name: "丙"}}, Duration: 200000}},
	}}
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()

	// The second of two artists credited.
	if err := tt.Click("乙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.nowPlaying || a.route != (route{page: pageArtist, id: "12"}) {
		t.Fatalf("after the click: Now Playing open %v, page %+v", a.nowPlaying, a.route)
	}

	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("丙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageArtist, id: "13"}) {
		t.Fatalf("a row's artist opened %+v", a.route)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != duet.ID {
		t.Errorf("a click on a row's artist played %+v", cur)
	}

	// Back goes to the artist before.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route.id != "12" {
		t.Errorf("back went to %+v", a.route)
	}
}
