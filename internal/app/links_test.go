package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// Links to Spotify read as its apps share them, with their tracking, in a
// language, or as URIs; anything else is a search.
func TestSpotifyLink(t *testing.T) {
	for _, c := range []struct{ in, kind, id string }{
		{"https://open.spotify.com/playlist/37i9dQZF1FbGLCAJ1AxBzw?si=9bL3TJ_lSGC3aUTVRlhAyA&utm_source=copy-link", "playlist", "37i9dQZF1FbGLCAJ1AxBzw"},
		{" https://open.spotify.com/intl-ja/album/3dJVHANUesvpD3fz1Ilm7I ", "album", "3dJVHANUesvpD3fz1Ilm7I"},
		{"spotify:artist:1iCsnjNikUpDDGDAxBalTs", "artist", "1iCsnjNikUpDDGDAxBalTs"},
		{"spotify:user:me:collection", "", ""},
		{"https://example.com/playlist/x", "", ""},
		{"春日影", "", ""},
	} {
		kind, id, ok := spotifyLink(c.in)
		if ok != (c.kind != "") || kind != c.kind || id != c.id {
			t.Errorf("spotifyLink(%q) = %q, %q, %v", c.in, kind, id, ok)
		}
	}
}

// A link to a mix of Spotify's, typed in the search field, keeps it among
// those the home page shows and plays it, which is all the app can do
// with one; an album's link opens the album. Neither searches.
func TestSearchFieldOpensLinks(t *testing.T) {
	a, rec := recordingApp()
	asked := fakeSearch(a)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("https://open.spotify.com/playlist/37i9dQZF1FbGLCAJ1AxBzw?si=9bL3TJ_lSGC3aUTVRlhAyA")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	uri := "spotify:playlist:37i9dQZF1FbGLCAJ1AxBzw"
	if req := rec.played(t); req.Context != uri {
		t.Fatalf("played %q", req.Context)
	}
	if len(a.mixes.list) != 1 || a.mixes.list[0].URI != uri {
		t.Fatalf("mixes %+v", a.mixes.list)
	}

	a.query = ""
	tt.Frame()
	tt.Type("https://open.spotify.com/album/3dJVHANUesvpD3fz1Ilm7I")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.route != (route{page: pageAlbum, id: "3dJVHANUesvpD3fz1Ilm7I"}) {
		t.Fatalf("the album's link opened %+v", a.route)
	}
	select {
	case r := <-asked:
		t.Fatalf("searched %+v", r)
	default:
	}
}
