package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// A click on a song's row plays it only if the listener chose so; else
// it takes a double click, as in Spotify's own app. A double click once
// played the song twice.
func TestRowPlaysOnClickIfChosen(t *testing.T) {
	for _, clickPlays := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.settings.ClickPlays = clickPlays
		a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: []music.Track{
			{ID: "1", Name: "一张相片", Artists: []music.Artist{{Name: "林宥嘉"}}, Duration: 200000},
			{ID: "2", Name: "宝船", Artists: []music.Artist{{Name: "DOUDOU"}}, Duration: 200000},
		}}
		a.route = route{page: pagePlaylist, id: "42"}
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()

		if err := tt.Click("宝船"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		played := a.pl.Current() != nil && a.pl.Current().ID == "2"
		if played != clickPlays {
			t.Errorf("with ClickPlays %v, a click played the song: %v", clickPlays, played)
		}
	}
}
