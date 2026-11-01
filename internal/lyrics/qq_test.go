package lyrics

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// qq is a QQ Music of a server that answers as answer does.
func qq(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *QQ {
	srv := httptest.NewServer(http.HandlerFunc(answer))
	t.Cleanup(srv.Close)
	return &QQ{Base: srv.URL, Lyric: srv.URL}
}

// qqSongs answers a search with songs, as QQ Music does.
func qqSongs(w http.ResponseWriter, songs ...map[string]any) {
	reply(w, map[string]any{"code": 0, qqSearch: map[string]any{
		"code": 0,
		"data": map[string]any{"body": map[string]any{"item_song": songs}},
	}})
}

func qqSearched(mid, album string, interval time.Duration) map[string]any {
	return map[string]any{
		"mid": mid, "name": "春日影", "title": "春日影",
		"interval": float64(interval / time.Second),
		"album":    map[string]any{"name": album},
		"singer":   []any{map[string]any{"name": "MyGO!!!!!"}},
	}
}

// The lyrics of the track as long are asked for by its mid, and come
// base64-encoded, with their HTML entities written out, and translated.
func TestQQFind(t *testing.T) {
	var mid atomic.Value
	q := qq(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/musicu.fcg":
			qqSongs(w,
				qqSearched("short", "迷跡波", 190*time.Second),
				qqSearched("right", "迷跡波", 258*time.Second),
			)
		case "/lyric/fcgi-bin/fcg_query_lyric_new.fcg":
			r.ParseForm()
			mid.Store(r.PostForm.Get("songmid"))
			reply(w, map[string]any{
				"retcode": 0,
				"code":    0,
				"lyric":   base64.StdEncoding.EncodeToString([]byte("[00:01.00]春日影&#58; 春\n")),
				"trans":   base64.StdEncoding.EncodeToString([]byte("[00:01.00]春日之影\n")),
			})
		default:
			http.NotFound(w, r)
		}
	})
	lines, err := found(q, spring)
	if err != nil || len(lines) != 1 {
		t.Fatalf("found %+v, %v", lines, err)
	}
	if got, _ := mid.Load().(string); got != "right" {
		t.Fatalf("asked for %q", got)
	}
	if lines[0].Text != "春日影: 春" || lines[0].Translation != "春日之影" {
		t.Fatalf("found %+v", lines[0])
	}
}

// A track QQ Music has no lyrics of, as it answers -1901, has none, not
// an error.
func TestQQNoLyrics(t *testing.T) {
	q := qq(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/musicu.fcg" {
			qqSongs(w, qqSearched("right", "迷跡波", 258*time.Second))
			return
		}
		reply(w, map[string]any{"retcode": -1901, "code": -1901})
	})
	if lines, err := found(q, spring); err != nil || len(lines) != 0 {
		t.Fatalf("found %+v, %v", lines, err)
	}
}

// QQ Music refusing a request is an error: the track wasn't asked for,
// and must not be kept as having no lyrics.
func TestQQRefused(t *testing.T) {
	q := qq(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"code": -1})
	})
	if _, err := found(q, spring); err == nil {
		t.Fatal("no error when refused")
	}
}
