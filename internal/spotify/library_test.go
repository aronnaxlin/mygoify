package spotify

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
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

// likedServer answers for the listener "me" and the liked songs in ids,
// the latest first, noting the calls.
func likedServer(t *testing.T, ids *[]string, calls *[]string) *Client {
	var mu sync.Mutex
	return client(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		*calls = append(*calls, r.URL.RequestURI())
		switch r.URL.Path {
		case "/me":
			w.Write([]byte(`{"id":"me"}`))
		case "/me/tracks":
			q := r.URL.Query()
			limit, _ := strconv.Atoi(q.Get("limit"))
			offset, _ := strconv.Atoi(q.Get("offset"))
			var items []string
			for _, id := range (*ids)[min(offset, len(*ids)):min(offset+limit, len(*ids))] {
				items = append(items, fmt.Sprintf(`{"track":{"id":%q,"type":"track","name":%q}}`, id, id))
			}
			next := "null"
			if offset+limit < len(*ids) {
				next = fmt.Sprintf(`"%s/me/tracks?limit=%d&offset=%d"`, BaseURL, limit, offset+limit)
			}
			fmt.Fprintf(w, `{"items":[%s],"next":%s,"total":%d}`, strings.Join(items, ","), next, len(*ids))
		default:
			t.Errorf("asked for %s", r.URL)
		}
	})
}

// The liked songs kept from before are read only since then, which takes
// a call rather than one a page, at each start; when they no longer add
// up, as when a song was unliked in another app, all are read again,
// from the page read already.
func TestLikedSince(t *testing.T) {
	ids := make([]string, 120)
	for i := range ids {
		ids[i] = fmt.Sprint("t", i)
	}
	var calls []string
	c := likedServer(t, &ids, &calls)
	cache := t.TempDir()
	read := func() []string {
		t.Helper()
		calls = nil
		got, err := (&Library{API: c, Cache: cache}).LikedIDs()
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := read(); !slices.Equal(got, ids) || len(calls) != 1+3 {
		t.Fatalf("first: %d songs after %q", len(got), calls)
	}
	ids = append([]string{"new"}, ids...)
	if got := read(); !slices.Equal(got, ids) || len(calls) != 1+1 {
		t.Fatalf("one liked since: %d songs, %v first, after %q", len(got), got[:1], calls)
	}
	ids = append([]string{"newer"}, slices.Delete(ids, 60, 62)...)
	if got := read(); !slices.Equal(got, ids) || len(calls) != 1+3 {
		t.Fatalf("two unliked elsewhere: %d songs after %q", len(got), calls)
	}
}

// The hearts and the liked songs' playlist, asking at once, read the
// liked songs once.
func TestLikedReadOnce(t *testing.T) {
	ids := make([]string, 120)
	for i := range ids {
		ids[i] = fmt.Sprint("t", i)
	}
	var calls []string
	c := likedServer(t, &ids, &calls)
	l := &Library{API: c}
	var wg sync.WaitGroup
	wg.Go(func() { l.LikedIDs() })
	wg.Go(func() { l.Playlist(likedID) })
	wg.Wait()
	var reads int
	for _, u := range calls {
		if strings.HasPrefix(u, "/me/tracks") {
			reads++
		}
	}
	if reads != 3 {
		t.Fatalf("read %d pages of 3: %q", reads, calls)
	}
}

// A playlist kept from before is read in one call while its snapshot is
// the same, and anew once it changed; while Spotify limits the app, it
// shows as kept.
func TestPlaylistKept(t *testing.T) {
	snapshot, limited := "s1", false
	var items int
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if limited {
			w.Header().Set("Retry-After", "20000")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		switch r.URL.Path {
		case "/me":
			w.Write([]byte(`{"id":"me"}`))
		case "/playlists/p":
			fmt.Fprintf(w, `{"id":"p","name":"睡前","owner":{"id":"me"},"snapshot_id":%q,"items":{"total":1}}`, snapshot)
		case "/playlists/p/items":
			items++
			fmt.Fprintf(w, `{"items":[{"item":{"id":%q,"type":"track","name":"一"}}],"next":null}`, snapshot)
		default:
			t.Errorf("asked for %s", r.URL)
		}
	})
	cache := t.TempDir()
	read := func() string {
		t.Helper()
		p, err := (&Library{API: c, Cache: cache}).Playlist("p")
		if err != nil || len(p.Tracks) != 1 {
			t.Fatalf("got %+v, %v", p, err)
		}
		return p.Tracks[0].ID
	}
	read()
	if got := read(); got != "s1" || items != 1 {
		t.Fatalf("same snapshot: %s, items read %d times", got, items)
	}
	snapshot = "s2"
	if got := read(); got != "s2" || items != 2 {
		t.Fatalf("new snapshot: %s, items read %d times", got, items)
	}
	limited = true
	if got := read(); got != "s2" {
		t.Fatalf("limited: %s", got)
	}
}
