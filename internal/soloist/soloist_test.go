package soloist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"mygoify/internal/library"
	"mygoify/internal/music"
	"mygoify/internal/playback"
	"mygoify/internal/spotify"
)

// fakeSoloist is a WebSocket that speaks as Soloist does: it tells what
// it was sent, and sends what the test gives it.
type fakeSoloist struct {
	dir      string
	srv      *httptest.Server
	commands chan map[string]any
	mu       sync.Mutex
	conn     *websocket.Conn
	conns    int // connections made
}

func newFakeSoloist(t *testing.T) *fakeSoloist {
	f := &fakeSoloist{dir: t.TempDir(), commands: make(chan map[string]any, 64)}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conn = conn
		f.conns++
		f.mu.Unlock()
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var cmd map[string]any
			json.Unmarshal(data, &cmd)
			f.commands <- cmd
		}
	}))
	t.Cleanup(f.srv.Close)
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(f.srv.URL, "http://"))
	os.WriteFile(filepath.Join(f.dir, "ws.addr"), []byte(host+"\n"), 0o600)
	os.WriteFile(filepath.Join(f.dir, "ws.port"), []byte(port+"\n"), 0o600)
	return f
}

func (f *fakeSoloist) send(t *testing.T, ev string) {
	t.Helper()
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	if err := conn.Write(context.Background(), websocket.MessageText, []byte(ev)); err != nil {
		t.Fatal(err)
	}
}

// next returns the next command sent, other than the queries Backend
// makes as it connects.
func (f *fakeSoloist) next(t *testing.T) map[string]any {
	t.Helper()
	for {
		select {
		case cmd := <-f.commands:
			switch cmd["command"] {
			case "get_auth_state", "get_state", "get_queue":
				continue
			}
			return cmd
		case <-time.After(2 * time.Second):
			t.Fatal("no command")
			return nil
		}
	}
}

// connected waits for Backend to have connected n times and asked for
// the queue, the last of what it asks as it connects.
func (f *fakeSoloist) connected(t *testing.T, n int) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		select {
		case cmd := <-f.commands:
			f.mu.Lock()
			conns := f.conns
			f.mu.Unlock()
			if cmd["command"] == "get_queue" && conns >= n {
				return
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatalf("not connected %d times", n)
}

func track(id, name string) string {
	return fmt.Sprintf(`{"uri":"spotify:track:%s","entity_type":"track","decorations":{"identity":{"name":%q},`+
		`"visual_identity":{"cover":[{"url":"https://i.scdn.co/image/s%s","size":"small"},{"url":"https://i.scdn.co/image/d%s","size":"default"},{"url":"https://i.scdn.co/image/l%s","size":"large"},{"url":"https://i.scdn.co/image/l%s","size":"xlarge"}]},`+
		`"parent":{"entity":{"uri":"spotify:album:al%s","entity_type":"album","decorations":{"identity":{"name":"专辑%s"}}}},`+
		`"creators":[{"entity":{"uri":"spotify:artist:ar1","entity_type":"artist","decorations":{"identity":{"name":"甲"}}}},{"entity":{"uri":"spotify:artist:ar2","entity_type":"artist","decorations":{"identity":{"name":"乙"}}}}],`+
		`"playback":{"duration_ms":200000}}}`, id, name, id, id, id, id, id, id)
}

func watch(b *Backend) chan playback.State {
	ch := make(chan playback.State, 64)
	b.Watch(func(st playback.State) { ch <- st })
	return ch
}

func until(t *testing.T, ch chan playback.State, ok func(playback.State) bool) playback.State {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case st := <-ch:
			if ok(st) {
				return st
			}
		case <-timeout:
			t.Fatal("no such state")
		}
	}
}

// Soloist's state and queue make the app's: what played, oldest first,
// then the track playing and what comes next, with their artists, album
// and covers by width, and where the track was at the time Soloist says.
func TestState(t *testing.T) {
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir})
	states := watch(b)
	f.connected(t, 1)
	f.send(t, `{"type":"playback_state","status":"playing","item":`+track("cur", "现在")+`,"context":{"uri":"spotify:album:x"},`+
		`"position":{"position_ms":30000,"timestamp_ms":1793408928492,"speed":1},"is_active":true,"options":{"shuffle":true,"repeat":"track"}}`)
	f.send(t, `{"type":"queue_changed","previous":[{"uid":"p1","source":"context","item":`+track("prev1", "刚才")+`},{"uid":"p2","source":"context","item":`+track("prev2", "更早")+`}],`+
		`"upcoming":[{"uid":"n1","source":"queue","item":`+track("next1", "下一首")+`}]}`)
	st := until(t, states, func(st playback.State) bool { return len(st.Queue) == 4 })
	var names []string
	for _, tr := range st.Queue {
		names = append(names, tr.Name)
	}
	if got := strings.Join(names, ","); got != "更早,刚才,现在,下一首" || st.At != 2 {
		t.Fatalf("queue %s, at %d", got, st.At)
	}
	cur := st.Queue[st.At]
	if cur.ID != "cur" || cur.Album.ID != "alcur" || len(cur.Artists) != 2 || cur.Artists[1].Name != "乙" || cur.Length() != 200*time.Second {
		t.Fatalf("track %+v", cur)
	}
	if len(cur.Album.Images) != 3 || cur.Album.Images.URL(300) != "https://i.scdn.co/image/dcur" || cur.Album.Images.URL(600) != "https://i.scdn.co/image/lcur" {
		t.Fatalf("covers %+v", cur.Album.Images)
	}
	if !st.Playing || st.Loading || !st.Shuffle || st.Repeat != playback.RepeatOne ||
		st.Position != 30*time.Second || !st.Since.Equal(time.UnixMilli(1793408928492)) {
		t.Fatalf("state %+v", st)
	}
	f.send(t, `{"type":"playback_changed","status":"buffering"}`)
	until(t, states, func(st playback.State) bool { return st.Playing && st.Loading })
	f.send(t, `{"type":"playback_state","status":"idle","item":{"uri":"","entity_type":""}}`)
	until(t, states, func(st playback.State) bool { return st.At == -1 && len(st.Queue) == 0 })
}

// A command that plays makes a Soloist that isn't the device playing so,
// and the others, queueing a track among them, don't take playback from
// where it is; repeat sets both of Soloist's.
func TestCommands(t *testing.T) {
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir})
	watch(b)
	f.connected(t, 1)
	if err := b.SetVolume(0.5); err != nil {
		t.Fatal(err)
	}
	if cmd := f.next(t); cmd["command"] != "set_volume" || cmd["volume"] != 50.0 {
		t.Fatalf("sent %v", cmd)
	}
	if err := b.AddToQueue(music.Track{ID: "4uLU6hMCjMI75M1A2tKUQC"}); err != nil {
		t.Fatal(err)
	}
	if cmd := f.next(t); cmd["command"] != "add_to_queue" || cmd["uri"] != "spotify:track:4uLU6hMCjMI75M1A2tKUQC" {
		t.Fatalf("sent %v", cmd)
	}
	resumed := make(chan error, 1)
	go func() { resumed <- b.Resume() }()
	if cmd := f.next(t); cmd["command"] != "activate" {
		t.Fatalf("played without activating: %v", cmd)
	}
	// Not until Soloist says it is the device playing: it ignored the
	// play sent at once, and the app's play button did nothing.
	select {
	case cmd := <-f.commands:
		t.Fatalf("sent %v before Soloist was active", cmd)
	case <-time.After(200 * time.Millisecond):
	}
	f.send(t, `{"type":"device_changed","is_active":true,"device_name":"MyGoify"}`)
	if cmd := f.next(t); cmd["command"] != "play" {
		t.Fatalf("sent %v", cmd)
	}
	if err := <-resumed; err != nil {
		t.Fatal(err)
	}
	b.SetRepeat(playback.RepeatOne)
	if a, c := f.next(t), f.next(t); a["command"] != "set_repeat_context" || a["enabled"] != false || c["command"] != "set_repeat_track" || c["enabled"] != true {
		t.Fatalf("sent %v then %v", a, c)
	}
}

// Before it connects, setting the volume is no failure: Soloist keeps its
// own. It was shown as playback failing as the app started.
func TestVolumeBeforeConnecting(t *testing.T) {
	b := New(Config{DataDir: t.TempDir()})
	if err := b.SetVolume(0.8); err != nil {
		t.Fatal(err)
	}
}

// Soloist restarting, the backend connects again and asks where
// playback is.
func TestReconnects(t *testing.T) {
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir})
	watch(b)
	f.connected(t, 1)
	f.mu.Lock()
	f.conn.CloseNow()
	f.mu.Unlock()
	f.connected(t, 2)
}

// Playing from a track of a context asks the Web API to play the context
// from it on this Soloist's device, found by its name, and found again
// when the Web API no longer knows the one found.
func TestPlay(t *testing.T) {
	var mu sync.Mutex
	var plays []string
	devices := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/me/player/devices":
			devices++
			fmt.Fprintf(w, `{"devices":[{"id":"phone","name":"手机"},{"id":"dev%d","name":"MyGoify"}]}`, devices)
		case "/me/player/play":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			b, _ := json.Marshal(body)
			plays = append(plays, r.URL.Query().Get("device_id")+" "+string(b))
			if len(plays) == 1 {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":{"status":404,"message":"Device not found"}}`))
			}
		}
	}))
	defer api.Close()
	base := spotify.BaseURL
	spotify.BaseURL = api.URL
	defer func() { spotify.BaseURL = base }()

	cfg := Config{DeviceName: "MyGoify", API: &spotify.Client{Token: token}, DeviceFile: filepath.Join(t.TempDir(), "device")}
	req := playback.Request{Context: "spotify:playlist:pl", Tracks: tracks("a", "b", "c"), Start: 2}
	if err := New(cfg).Play(req); err != nil {
		t.Fatal(err)
	}
	// The app started again plays on the device found, without listing
	// devices: each list was a call to the Web API as the app started.
	if err := New(cfg).Play(req); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		`dev1 {"context_uri":"spotify:playlist:pl","offset":{"uri":"spotify:track:c"}}`,
		`dev2 {"context_uri":"spotify:playlist:pl","offset":{"uri":"spotify:track:c"}}`,
		`dev2 {"context_uri":"spotify:playlist:pl","offset":{"uri":"spotify:track:c"}}`,
	}
	if strings.Join(plays, "\n") != strings.Join(want, "\n") || devices != 2 {
		t.Fatalf("played:\n%s\nafter %d lists of devices", strings.Join(plays, "\n"), devices)
	}
}

func token(context.Context) (string, error) { return "tok", nil }

// tracks are the tracks of ids, as Soloist names them.
func tracks(ids ...string) []music.Track {
	var ts []music.Track
	for _, id := range ids {
		var e entity
		json.Unmarshal([]byte(track(id, id)), &e)
		ts = append(ts, e.track())
	}
	return ts
}

// webAPI answers each call to the Web API with status, and counts them.
func webAPI(t *testing.T, status int) *int {
	var mu sync.Mutex
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "3600")
		}
		w.WriteHeader(status)
		fmt.Fprint(w, `{"devices":[{"id":"dev","name":"MyGoify"}]}`)
	}))
	t.Cleanup(api.Close)
	base := spotify.BaseURL
	spotify.BaseURL = api.URL
	t.Cleanup(func() { spotify.BaseURL = base })
	return &calls
}

// active connects b to f as the device playing.
func active(t *testing.T, f *fakeSoloist, b *Backend) {
	t.Helper()
	states := watch(b)
	f.connected(t, 1)
	f.send(t, `{"type":"device_changed","is_active":true}`)
	until(t, states, func(playback.State) bool { b.mu.Lock(); defer b.mu.Unlock(); return b.active })
}

// Playing an album or a playlist from its start, as its play button does,
// asks Soloist alone, though tracks the listener can't play come first:
// the Web API need not hear of it.
func TestPlayFromStart(t *testing.T) {
	calls := webAPI(t, http.StatusOK)
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir, DeviceName: "MyGoify", API: &spotify.Client{Token: token}})
	active(t, f, b)
	ts := tracks("a", "b", "c")
	ts[0].Unplayable = true
	if err := b.Play(playback.Request{Context: "spotify:album:al", Tracks: ts, Start: 1}); err != nil {
		t.Fatal(err)
	}
	if cmd := f.next(t); cmd["command"] != "play" || cmd["uri"] != "spotify:album:al" {
		t.Fatalf("sent %v", cmd)
	}
	if *calls != 0 {
		t.Fatalf("%d calls to the Web API", *calls)
	}
}

// While Spotify limits the app's calls, playing from a track of a context
// fails, saying until when, and asks Soloist nothing: played as the track
// alone, with those after it queued, it changed what the listener's other
// devices showed playing, and left their queue full.
func TestPlayLimited(t *testing.T) {
	webAPI(t, http.StatusTooManyRequests)
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir, DeviceName: "MyGoify", API: &spotify.Client{Token: token}})
	active(t, f, b)
	err := b.Play(playback.Request{Context: "spotify:album:al", Tracks: tracks("a", "b", "c"), Start: 1})
	if !errors.Is(err, library.ErrLimited) {
		t.Fatalf("played, limited: %v", err)
	}
	select {
	case cmd := <-f.commands:
		t.Fatalf("sent %v", cmd)
	case <-time.After(200 * time.Millisecond):
	}
}

// Onboarding goes on to its last step once Soloist says it is logged in,
// and not as it says it isn't.
func TestLoggedInIsTold(t *testing.T) {
	f := newFakeSoloist(t)
	told := make(chan struct{}, 4)
	b := New(Config{DataDir: f.dir, LoggedIn: func() { told <- struct{}{} }})
	watch(b)
	f.connected(t, 1)
	f.send(t, `{"type":"auth_state","logged_in":false}`)
	select {
	case <-told:
		t.Fatal("told of a login as Soloist wasn't logged in")
	case <-time.After(100 * time.Millisecond):
	}
	f.send(t, `{"type":"auth_state","logged_in":true}`)
	select {
	case <-told:
	case <-time.After(2 * time.Second):
		t.Fatal("not told of the login")
	}
}
