package soloist

import (
	"net/http"
	"testing"

	"mygoify/internal/music"
	"mygoify/internal/playback"
	"mygoify/internal/spotify"
)

// stationOf is the radio of the track id.
func stationOf(id string) string { return music.StationURI("spotify:track:" + id) }

// expect fails unless the next command is name, with uri or volume if
// given.
func expect(t *testing.T, f *fakeSoloist, name string, arg ...any) {
	t.Helper()
	cmd := f.next(t)
	if cmd["command"] != name {
		t.Fatalf("sent %v, want %s %v", cmd, name, arg)
	}
	if len(arg) == 2 && cmd[arg[0].(string)] != arg[1] {
		t.Fatalf("sent %v, want %s %v", cmd, name, arg)
	}
}

// A song's radio plays from Soloist, shuffled or not, as the Web API
// refuses it, and starts with the song: once Soloist starts the radio
// with another, the song is queued and skipped to, muted meanwhile.
// Skipping at once moved on to the radio's second song (2026-11-06).
func TestSongRadioStartsWithTheSong(t *testing.T) {
	calls := webAPI(t, http.StatusOK)
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir, DeviceName: "MyGoify", API: &spotify.Client{Token: token}})
	active(t, f, b)
	f.send(t, `{"type":"device_changed","is_active":true,"options":{"shuffle":true,"repeat":"off"}}`)
	b.SetVolume(0.5)
	expect(t, f, "set_volume", "volume", float64(50))

	done := make(chan error, 1)
	go func() { done <- b.Play(playback.Request{Context: stationOf("k"), Tracks: tracks("k"), Start: 0}) }()
	expect(t, f, "play", "uri", stationOf("k"))
	expect(t, f, "set_volume", "volume", float64(0))
	select {
	case cmd := <-f.commands:
		t.Fatalf("sent %v before the radio started", cmd)
	default:
	}
	f.send(t, `{"type":"context_changed","context":{"uri":"`+stationOf("k")+`","entity_type":"unknown","decorations":{}}}`)
	f.send(t, `{"type":"playback_state","status":"playing","item":`+track("x", "另一首")+`}`)
	f.send(t, `{"type":"queue_changed","previous":[],"upcoming":[{"uid":"y","source":"context","item":`+track("y", "y")+`}]}`)
	expect(t, f, "add_to_queue", "uri", "spotify:track:k")
	expect(t, f, "skip_next")
	f.send(t, `{"type":"playback_state","status":"playing","item":`+track("k", "KISS")+`}`)
	expect(t, f, "set_volume", "volume", float64(50))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if *calls != 0 {
		t.Errorf("%d calls to the Web API", *calls)
	}
}

// With songs the listener queued, a song's radio starts as Soloist starts
// it: skipping to the song would skip theirs.
func TestSongRadioLeavesTheQueue(t *testing.T) {
	webAPI(t, http.StatusOK)
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir, DeviceName: "MyGoify", API: &spotify.Client{Token: token}})
	active(t, f, b)
	b.SetVolume(0.5)
	expect(t, f, "set_volume")

	done := make(chan error, 1)
	go func() { done <- b.Play(playback.Request{Context: stationOf("k"), Tracks: tracks("k"), Start: 0}) }()
	expect(t, f, "play")
	expect(t, f, "set_volume", "volume", float64(0))
	f.send(t, `{"type":"queue_changed","previous":[],"upcoming":[{"uid":"q","source":"queue","item":`+track("q", "q")+`}]}`)
	f.send(t, `{"type":"playback_state","status":"playing","item":`+track("x", "x")+`,"context":{"uri":"`+stationOf("k")+`"}}`)
	expect(t, f, "set_volume", "volume", float64(50))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// Within a radio, a song further down the queue is skipped to: the Web
// API, which plays a context from a track, refuses a radio.
func TestJumpWithinRadioSkips(t *testing.T) {
	calls := webAPI(t, http.StatusOK)
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir, DeviceName: "MyGoify", API: &spotify.Client{Token: token}})
	active(t, f, b)
	f.send(t, `{"type":"queue_changed","previous":[],"upcoming":[{"uid":"b","source":"context","item":`+track("b", "b")+`},{"uid":"c","source":"context","item":`+track("c", "c")+`}]}`)
	f.send(t, `{"type":"playback_state","status":"playing","item":`+track("a", "a")+`,"context":{"uri":"spotify:station:artist:ar1"}}`)
	b.waitFor(stationWait, func() bool { return b.ctxURI != "" && len(b.upcoming) == 2 })
	if err := b.Jump(2); err != nil {
		t.Fatal(err)
	}
	expect(t, f, "skip_next")
	expect(t, f, "skip_next")
	if *calls != 0 {
		t.Errorf("%d calls to the Web API", *calls)
	}
}
