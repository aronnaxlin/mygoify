package app

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

const (
	dailyMix = "spotify:playlist:37i9dQZF1E39i7UAVcxDHE"
	daylist  = "spotify:playlist:37i9dQZF1EP6YuccBxUcC1"
	weekly   = "spotify:playlist:37i9dQZEVXcJIKCgVG2l9T"
)

func mixURIs(a *app) []string {
	var uris []string
	for _, m := range a.mixes.list {
		uris = append(uris, m.URI)
	}
	return uris
}

// The mixes the listener played lately come first, the latest first, and
// those seen before keep their place, so a card clicked stays under the
// pointer; the listener's own playlists and Spotify's editorial ones
// aren't theirs.
func TestMixesOrder(t *testing.T) {
	a := newTestApp()
	now := time.Now()
	a.seeHomeMixes([]string{dailyMix, weekly}, now)
	a.seeMix(daylist, "", now.Add(time.Minute))
	a.seeMix(weekly, "", now.Add(2*time.Minute))
	a.seeMix("spotify:playlist:1g41u90mDfHOfbFPVNBava", "", now)
	a.seeMix("spotify:playlist:37i9dQZF1DXcBWIGoYBM5M", "", now)
	if got, want := mixURIs(a), []string{daylist, dailyMix, weekly}; !slices.Equal(got, want) {
		t.Fatalf("mixes %q, want %q", got, want)
	}
}

// A mix shows by the name Soloist gave it as it played, the daylist's of
// the time of day, rather than oEmbed's, which calls it "daylist".
func TestMixNamedAsPlayed(t *testing.T) {
	a := newTestApp()
	a.mixes.list = []mix{{URI: daylist, Title: "daylist", Asked: time.Now(), Seen: time.Now()}}
	a.seeMix(daylist, "k-indie mori kei sunday afternoon", time.Now())
	if got := a.mixes.list[0].name(); got != "k-indie mori kei sunday afternoon" {
		t.Fatalf("the daylist shows as %q", got)
	}
}

// A mix's card on the home page plays the mix from its start, from a
// click anywhere on it, as the Web API can't list its songs.
func TestMixCardPlays(t *testing.T) {
	a, rec := recordingApp()
	a.home = music.Home{Recent: []music.Track{{ID: "r", Name: "最近"}}}
	a.mixes.list = []mix{{URI: dailyMix, Title: "Daily Mix 3", Asked: time.Now(), Seen: time.Now()}}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.Click("Daily Mix 3"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	req := rec.played(t)
	if req.Context != dailyMix || len(req.Tracks) != 0 {
		t.Fatalf("played %q from %+v", req.Context, req.Tracks)
	}
}
