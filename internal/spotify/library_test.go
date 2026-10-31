package spotify

import (
	"net/http"
	"strings"
	"testing"
)

// A playlist's tracks are read under item, or track, its old name, but
// for local files, which Spotify streams not; someone else's, which the
// Web API refuses to list, is hidden rather than failing.
func TestPlaylist(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/playlists/mine":
			w.Write([]byte(`{"id":"mine","name":"睡前","owner":{"id":"me","display_name":"我"},"items":{"total":3}}`))
		case r.URL.Path == "/playlists/mine/items":
			w.Write([]byte(`{"items":[` +
				`{"item":{"id":"a","type":"track","name":"一","artists":[{"id":"ar","name":"甲"}],"album":{"id":"al","name":"专辑","images":[{"url":"u","width":300,"height":300}]},"duration_ms":1000}},` +
				`{"track":{"id":"b","type":"track","name":"二","duration_ms":2000}},` +
				`{"is_local":true,"item":{"id":"","type":"track","name":"本地","is_local":true}}` +
				`],"next":null}`))
		case r.URL.Path == "/playlists/theirs":
			w.Write([]byte(`{"id":"theirs","name":"别人的","owner":{"id":"them"},"items":{"total":50}}`))
		case r.URL.Path == "/playlists/theirs/items":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":{"status":403,"message":"Forbidden"}}`))
		default:
			t.Errorf("asked for %s", r.URL)
		}
	})
	l := &Library{API: c}
	p, err := l.Playlist("mine")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tr := range p.Tracks {
		names = append(names, tr.Name)
	}
	if strings.Join(names, ",") != "一,二" || p.Tracks[0].Album.Images.URL(300) != "u" || p.Hidden {
		t.Fatalf("tracks %v, %+v", names, p)
	}
	p, err = l.Playlist("theirs")
	if err != nil || !p.Hidden || p.TrackCount != 50 {
		t.Fatalf("someone else's: %+v, %v", p, err)
	}
}
