package spotify

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

// The home page finds the playlists Spotify made for the listener in what
// they played lately, the latest first, once each: not Spotify's editorial
// playlists, which are everyone's, nor the listener's own.
func TestHomeMixes(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/me/player/recently-played":
			item := func(id, ctx string) string {
				return `{"track":{"id":"` + id + `","type":"track","name":"` + id + `","duration_ms":1000},"context":` + ctx + `}`
			}
			pl := func(id string) string { return `{"type":"playlist","uri":"spotify:playlist:` + id + `"}` }
			w.Write([]byte(`{"items":[` +
				item("a", pl("37i9dQZF1E39i7UAVcxDHE")) + `,` + // a Daily Mix
				item("b", pl("37i9dQZF1DXcBWIGoYBM5M")) + `,` + // Today's Top Hits
				item("c", `null`) + `,` +
				item("d", pl("1g41u90mDfHOfbFPVNBava")) + `,` + // the listener's
				item("e", pl("37i9dQZF1E39i7UAVcxDHE")) + `,` +
				item("f", pl("37i9dQZF1EP6YuccBxUcC1")) + // the daylist
				`]}`))
		default:
			w.Write([]byte(`{"items":[]}`))
		}
	})
	h, err := (&Library{API: c}).Home()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"spotify:playlist:37i9dQZF1E39i7UAVcxDHE", "spotify:playlist:37i9dQZF1EP6YuccBxUcC1"}
	if !slices.Equal(h.Mixes, want) {
		t.Fatalf("mixes %q, want %q", h.Mixes, want)
	}
}

// A mix is named, with its cover, by Spotify's oEmbed, without the token,
// once a day: the Web API won't read it.
func TestMixFromOEmbed(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.URL.Query().Get("url")+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		w.Write([]byte(`{"title":"Daily Mix 3","thumbnail_url":"https://pickasso.spotifycdn.com/daily/3"}`))
	}))
	defer srv.Close()
	oe := OEmbedURL
	OEmbedURL = srv.URL
	defer func() { OEmbedURL = oe }()

	l := &Library{Cache: t.TempDir()}
	for range 2 {
		pl, err := l.Mix("37i9dQZF1E39i7UAVcxDHE")
		if err != nil {
			t.Fatal(err)
		}
		if pl.Name != "Daily Mix 3" || pl.Images.URL(300) != "https://pickasso.spotifycdn.com/daily/3" || !pl.Hidden {
			t.Fatalf("got %+v", pl)
		}
	}
	if want := []string{"https://open.spotify.com/playlist/37i9dQZF1E39i7UAVcxDHE "}; !slices.Equal(asked, want) {
		t.Fatalf("asked %q, want %q", asked, want)
	}
}
