package lyrics

import (
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// netease is a NetEase of a server that answers as answer does.
func netease(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *NetEase {
	srv := httptest.NewServer(http.HandlerFunc(answer))
	t.Cleanup(srv.Close)
	return &NetEase{Base: srv.URL, Eapi: srv.URL}
}

// neSongs answers a search with songs, as NetEase does.
func neSongs(w http.ResponseWriter, songs ...map[string]any) {
	reply(w, map[string]any{"code": 200, "result": map[string]any{"songs": songs}})
}

func neSearched(id int64, album string, duration time.Duration) map[string]any {
	return map[string]any{
		"id": id, "name": "春日影", "duration": float64(duration.Milliseconds()),
		"album":   map[string]any{"name": album},
		"artists": []any{map[string]any{"name": "MyGO!!!!!"}},
	}
}

// A track is searched for as it is named, then without what Spotify adds
// to it, and the lyrics asked for are of the track as long, of its album
// if several are; they come word-timed, with their translation.
func TestNetEaseFind(t *testing.T) {
	var lyricID atomic.Value
	n := netease(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search/get/web":
			if name := r.URL.Query().Get("s"); !strings.Contains(name, " - Remastered 2011") {
				// Of the wrong length: none is near.
				neSongs(w, neSearched(1, "迷跡波", 190*time.Second))
				return
			}
			neSongs(w,
				neSearched(2, "精选", 2573*time.Second),
				neSearched(3, "迷跡波", 257500*time.Millisecond),
			)
		case "/eapi/song/lyric/v1":
			lyricID.Store(eapiAsked(t, r))
			reply(w, map[string]any{
				"lrc":    map[string]any{"lyric": "[00:01.00]春日影\n"},
				"tlyric": map[string]any{"lyric": "[00:01.00]春日之影\n"},
				"yrc":    map[string]any{"lyric": "[1000,500](1000,500,0)春(1500,500,0)日影\n"},
			})
		default:
			http.NotFound(w, r)
		}
	})
	q := spring
	q.Name += " - Remastered 2011"
	lines, err := found(n, q)
	if err != nil || len(lines) != 1 {
		t.Fatalf("found %+v, %v", lines, err)
	}
	if id, _ := lyricID.Load().(string); id != "3" {
		t.Fatalf("asked for %q", id)
	}
	if !lines[0].Timed || lines[0].Translation != "春日之影" {
		t.Fatalf("found %+v", lines[0])
	}
}

// A track NetEase says it has no lyrics of has none, not an error.
func TestNetEaseNoLyrics(t *testing.T) {
	n := netease(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search/get/web" {
			neSongs(w, neSearched(3, "迷跡波", 257500*time.Millisecond))
			return
		}
		reply(w, map[string]any{"nolyric": true, "code": 200})
	})
	if lines, err := found(n, spring); err != nil || len(lines) != 0 {
		t.Fatalf("found %+v, %v", lines, err)
	}
}

// NetEase refusing the search, as it does outside China, is an error: the
// track wasn't asked for, and must not be kept as having no lyrics.
func TestNetEaseRefused(t *testing.T) {
	n := netease(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"code": 460, "msg": "Cheating"})
	})
	if _, err := found(n, spring); err == nil {
		t.Fatal("no error when refused")
	}
}

// What the desktop client's API is asked is signed and encrypted as it
// expects, as another implementation of it has it.
func TestEapiSign(t *testing.T) {
	got, err := eapiSign("/api/song/lyric/v1", map[string]string{"id": "1"})
	if err != nil {
		t.Fatal(err)
	}
	want := "04AE33D34A93FE3EC22DA8FA305D290AB337D0FE5F36D211DE0D338CC6AA89D0E8744000D04B3D7A3D495AEE86A48DC4BA9A6E5CBA7E957EED8B6484EC4A40A958A1A8DBDEAC922FD61D5CFFB632AD85F1A74DF512BBC164ECEFA314B2112E14"
	if got != want {
		t.Fatalf("signed %s", got)
	}
}

// eapiAsked returns the id asked for in the params of r, as they decrypt
// to the path, the JSON data, and their MD5.
func eapiAsked(t *testing.T, r *http.Request) string {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(r.PostForm.Get("params"))
	if err != nil || len(b)%aes.BlockSize != 0 {
		t.Fatalf("params %q", r.PostForm.Get("params"))
	}
	block, err := aes.NewCipher([]byte(eapiKey))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(b); i += aes.BlockSize {
		block.Decrypt(b[i:], b[i:])
	}
	// The path, the data, and the MD5 of the two, each between a dash.
	fields := strings.Split(string(b[:len(b)-int(b[len(b)-1])]), "-36cd479b6b5-")
	if len(fields) != 3 {
		t.Fatalf("signed %q", b)
	}
	var data struct {
		ID     string            `json:"id"`
		Header map[string]string `json:"header"`
	}
	if err := json.Unmarshal([]byte(fields[1]), &data); err != nil {
		t.Fatal(err)
	}
	if data.Header["os"] == "" {
		t.Fatal("no header")
	}
	return data.ID
}
