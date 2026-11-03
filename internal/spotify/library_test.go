package spotify

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"mygoify/internal/music"
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
		case "/me/library":
		case "/me/tracks":
			q := r.URL.Query()
			limit, _ := strconv.Atoi(q.Get("limit"))
			offset, _ := strconv.Atoi(q.Get("offset"))
			var items []string
			for _, id := range (*ids)[min(offset, len(*ids)):min(offset+limit, len(*ids))] {
				if strings.HasPrefix(id, "local") {
					// A local file, which the app skips: it still takes a
					// place in Spotify's list.
					items = append(items, fmt.Sprintf(`{"track":{"id":%q,"type":"track","name":%q,"is_local":true}}`, id, id))
					continue
				}
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

// names says which songs ts are, from the first to the last, or none.
func names(ts []music.Track) string {
	if len(ts) == 0 {
		return "none"
	}
	return ts[0].ID + ".." + ts[len(ts)-1].ID
}

// The liked songs of a large library show a page at a time: their
// playlist reads the first, and each page after it as the app asks, while
// songs liked since it was last read come on top of what was read before.
// Counted fewer than read, as when a song was unliked in another app, they
// start over from the first page.
func TestLikedPaged(t *testing.T) {
	ids := make([]string, 120)
	for i := range ids {
		ids[i] = fmt.Sprint("t", i)
	}
	// A local file among them: it takes a place in Spotify's list, and
	// none among what the app shows, which pages on past it.
	ids[10] = "local"
	var calls []string
	c := likedServer(t, &ids, &calls)
	cache := t.TempDir()
	open := func() (*Library, *music.Playlist) {
		t.Helper()
		calls = nil
		l := &Library{API: c, Cache: cache}
		p, err := l.Playlist(likedID)
		if err != nil {
			t.Fatal(err)
		}
		return l, p
	}
	l, p := open()
	if got := names(p.Tracks); len(p.Tracks) != 49 || got != "t0..t49" || p.TrackCount != 120 || !p.More || len(calls) != 2 {
		t.Fatalf("first page: %s, %d of %d songs, more %v, after %q", got, len(p.Tracks), p.TrackCount, p.More, calls)
	}
	next, more, err := l.LikedMore()
	if err != nil || names(next) != "t50..t99" || !more {
		t.Fatalf("next page: %s, more %v, %v", names(next), more, err)
	}
	// Read again, what was read shows at once: one call reads the top.
	l, p = open()
	if got := names(p.Tracks); len(p.Tracks) != 99 || !p.More || len(calls) != 2 {
		t.Fatalf("read again: %s, %d songs, more %v, after %q", got, len(p.Tracks), p.More, calls)
	}
	if next, more, err = l.LikedMore(); err != nil || names(next) != "t100..t119" || more {
		t.Fatalf("last page: %s, more %v, %v", names(next), more, err)
	}
	// A song liked since: it comes first, and what was read stays.
	ids = append([]string{"new"}, ids...)
	_, p = open()
	if got := names(p.Tracks); len(p.Tracks) != 120 || p.Tracks[0].ID != "new" || p.More || len(calls) != 2 {
		t.Fatalf("one liked since: %s, %d songs, more %v, after %q", got, len(p.Tracks), p.More, calls)
	}
	// Two unliked elsewhere: Spotify's list no longer holds what was read
	// where it was, so the first page comes again.
	ids = slices.Delete(ids, 60, 62)
	_, p = open()
	if got := names(p.Tracks); len(p.Tracks) != 49 || p.TrackCount != 119 || !p.More {
		t.Fatalf("two unliked elsewhere: %s, %d of %d songs, more %v", got, len(p.Tracks), p.TrackCount, p.More)
	}
}

// A song unliked among those read moves the songs after it up one in
// Spotify's list: the page after them starts one earlier, not past a song.
func TestLikedUnlikeMovesPages(t *testing.T) {
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = fmt.Sprint("t", i)
	}
	var calls []string
	c := likedServer(t, &ids, &calls)
	l := &Library{API: c}
	if p, err := l.Playlist(likedID); err != nil || len(p.Tracks) != 50 {
		t.Fatalf("first page: %d songs, %v", len(p.Tracks), err)
	}
	ids = slices.Delete(ids, 4, 5) // t4, unliked here
	if err := l.Like("t4", false); err != nil {
		t.Fatal(err)
	}
	next, more, err := l.LikedMore()
	if err != nil || names(next) != "t50..t99" || more {
		t.Fatalf("the page after: %s, more %v, %v", names(next), more, err)
	}
}

// The liked songs' playlist, asked for twice at once, reads its page once.
func TestLikedReadOnce(t *testing.T) {
	ids := make([]string, 120)
	for i := range ids {
		ids[i] = fmt.Sprint("t", i)
	}
	var calls []string
	c := likedServer(t, &ids, &calls)
	l := &Library{API: c}
	var wg sync.WaitGroup
	wg.Go(func() { l.Playlist(likedID) })
	wg.Go(func() { l.Playlist(likedID) })
	wg.Wait()
	var reads int
	for _, u := range calls {
		if strings.HasPrefix(u, "/me/tracks") {
			reads++
		}
	}
	if reads != 1 {
		t.Fatalf("read %d pages of 1: %q", reads, calls)
	}
}

// The hearts ask which of the songs shown the listener likes, in batches of
// as many as the Web API takes, each answered in its place.
func TestLikes(t *testing.T) {
	var requests int
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/library/contains" {
			t.Errorf("asked for %s", r.URL)
			return
		}
		requests++
		var got []string
		for _, uri := range strings.Split(r.URL.Query().Get("uris"), ",") {
			got = append(got, fmt.Sprint(strings.Contains(uri, ":liked")))
		}
		w.Write([]byte("[" + strings.Join(got, ",") + "]"))
	})
	ids := make([]string, 120)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%d", []string{"t", "liked"}[i%2], i)
	}
	likes, err := (&Library{API: c}).Likes(ids)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || len(likes) != len(ids) {
		t.Fatalf("%d requests for %d songs answered %d", requests, len(ids), len(likes))
	}
	for _, id := range ids {
		if got, want := likes[id], strings.HasPrefix(id, "liked"); got != want {
			t.Fatalf("%s: liked %v, want %v", id, got, want)
		}
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

// A search counts the playlists taken down, which Spotify lists as null,
// so that the next page starts past them: paged from what showed, the
// app showed results twice.
func TestSearchSkipped(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"playlists":{"items":[{"id":"a","name":"甲","owner":{"id":"x"}},null,{"id":"b","name":"乙","owner":{"id":"x"}}],"next":"more"}}`))
	})
	l := &Library{API: c}
	res, err := l.Search("春日影", music.SearchPlaylists, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Len() != 2 || res.Skipped != 1 || !res.More {
		t.Fatalf("got %d, skipped %d, more %v", res.Len(), res.Skipped, res.More)
	}
}
