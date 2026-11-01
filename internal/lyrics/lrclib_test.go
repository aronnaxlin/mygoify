package lyrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const song = "[00:12.00]春日影\n[00:15.50]悴んだ心 ふるえる眼差し\n"

// lrclib is an LRCLIB of a server that answers as answer does.
func lrclib(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) (*LRCLIB, *atomic.Int32) {
	asked := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		answer(w, r)
	}))
	t.Cleanup(srv.Close)
	return &LRCLIB{Base: srv.URL, pause: time.Millisecond}, asked
}

// findLines asks l for q's lyrics, as lines.
func findLines(t *testing.T, l *LRCLIB, q Query) ([]Line, error) {
	t.Helper()
	f, err := l.Find(context.Background(), q)
	return f.Lines(), err
}

func reply(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }

var spring = Query{ID: "spring", Name: "春日影", Artist: "MyGO!!!!!", Album: "迷跡波", Duration: 257500 * time.Millisecond}

// A track LRCLIB doesn't have under its album is searched for: of the
// same song, the one as long with timed lyrics, of its album if several
// are.
func TestFindSearches(t *testing.T) {
	l, _ := lrclib(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/get" {
			http.NotFound(w, r)
			return
		}
		reply(w, []record{
			{AlbumName: "春日影", Duration: 190, SyncedLyrics: "[00:01.00]TV size\n"},
			{AlbumName: "迷跡波", Duration: 258, SyncedLyrics: ""},
			{AlbumName: "Soundtrack", Duration: 257.6, SyncedLyrics: "[00:01.00]soundtrack\n"},
			{AlbumName: "迷跡波", Duration: 257.4, SyncedLyrics: song},
		})
	})
	lines, err := findLines(t, l, spring)
	if err != nil || len(lines) != 2 || lines[0].Text != "春日影" {
		t.Fatalf("found %+v, %v", lines, err)
	}
}

// A name Spotify adds to, as " - Remastered 2011", is searched for again
// without it.
func TestFindBareName(t *testing.T) {
	l, _ := lrclib(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("track_name") {
		case "Here Comes the Sun":
			if r.URL.Path == "/api/search" {
				reply(w, []record{{Duration: 185, SyncedLyrics: "[00:10.00]Here comes the sun\n"}})
				return
			}
		case "Here Comes the Sun - Remastered 2019":
			if r.URL.Path == "/api/search" {
				reply(w, []record{})
				return
			}
		}
		http.NotFound(w, r)
	})
	lines, err := findLines(t, l, Query{ID: "sun", Name: "Here Comes the Sun - Remastered 2019", Artist: "The Beatles", Duration: 185 * time.Second})
	if err != nil || len(lines) != 1 {
		t.Fatalf("found %+v, %v", lines, err)
	}
}

// A song Spotify names the artist of otherwise, as "Rene Liu" for 刘若英,
// is found by its name alone, if as long.
func TestFindOtherScript(t *testing.T) {
	l, _ := lrclib(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/get":
			http.NotFound(w, r)
		case r.URL.Query().Has("artist_name"):
			reply(w, []record{})
		default:
			reply(w, []record{
				{ArtistName: "刘若英", AlbumName: "我等你", Duration: 82, SyncedLyrics: "[00:01.00]片段\n"},
				{ArtistName: "刘若英", AlbumName: "我等你", Duration: 341, SyncedLyrics: song},
			})
		}
	})
	q := Query{ID: "later", Name: "后来", Artist: "Rene Liu", Album: "我等你", Duration: 341200 * time.Millisecond}
	if lines, err := findLines(t, l, q); err != nil || len(lines) != 2 {
		t.Fatalf("found %+v, %v", lines, err)
	}
	q.ID, q.Duration = "later-live", 343*time.Second
	if lines, err := findLines(t, l, q); err != nil || len(lines) != 0 {
		t.Fatalf("found %+v, %v of another length", lines, err)
	}
}

// A track LRCLIB says it is too busy to get is searched for at once, a
// search it is too busy for is asked again, and its being down isn't kept
// as the track having no lyrics.
func TestFindBusy(t *testing.T) {
	var busy atomic.Int32
	var gets atomic.Int32
	answer := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/get" {
			gets.Add(1)
			http.Error(w, `{"name":"ServerOverloaded"}`, http.StatusServiceUnavailable)
			return
		}
		if busy.Add(-1) >= 0 {
			http.Error(w, `{"name":"ServerOverloaded"}`, http.StatusServiceUnavailable)
			return
		}
		reply(w, []record{{AlbumName: "迷跡波", Duration: 257.5, SyncedLyrics: song}})
	}
	busy.Store(2)
	l, _ := lrclib(t, answer)
	if lines, err := findLines(t, l, spring); err != nil || len(lines) != 2 || gets.Load() != 1 {
		t.Fatalf("found %+v, %v, getting %d times", lines, err, gets.Load())
	}

	busy.Store(1 << 20)
	l, _ = lrclib(t, answer)
	if _, err := findLines(t, l, spring); err == nil {
		t.Fatal("no error while down")
	}
	busy.Store(0)
	if lines, err := findLines(t, l, spring); err != nil || len(lines) != 2 {
		t.Fatalf("found %+v, %v once up", lines, err)
	}
}

// A track without a name isn't looked for, nor kept: Soloist names the
// first track of a context another device plays only later. LRCLIB was
// asked for it, refused it, and was asked again and again.
func TestFindUnnamed(t *testing.T) {
	l, asked := lrclib(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})
	cache := t.TempDir()
	unnamed := spring
	unnamed.Name = ""
	if lines, err := (&Sources{All: []Source{l}, Cache: cache}).Find(context.Background(), unnamed); lines != nil || err != nil || asked.Load() != 0 {
		t.Fatalf("found %+v, %v, asking %d times", lines, err, asked.Load())
	}
	if _, err := os.Stat(filepath.Join(cache, unnamed.ID+".json")); err == nil {
		t.Error("kept a track without a name")
	}
}

// A track LRCLIB refuses to look for, answering 400, is kept as having no
// lyrics, rather than being asked for again and again as if LRCLIB were
// down.
func TestFindRefused(t *testing.T) {
	l, asked := lrclib(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})
	s := &Sources{All: []Source{l}, Cache: t.TempDir()}
	for range 2 {
		if lines, err := s.Find(context.Background(), spring); lines != nil || err != nil {
			t.Fatalf("found %+v, %v", lines, err)
		}
	}
	if n := asked.Load(); n != 1 {
		t.Errorf("asked %d times, want once", n)
	}
}
