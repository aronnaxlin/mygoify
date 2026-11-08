package app

import (
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
	"mygoify/internal/playback"
)

// recorder plays as the fake backend does, and keeps what it was asked to
// play.
type recorder struct {
	*playback.Fake
	mu   sync.Mutex
	reqs []playback.Request
}

func (r *recorder) Play(req playback.Request) error {
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mu.Unlock()
	return r.Fake.Play(req)
}

// played waits for the backend to be asked to play, and returns what.
func (r *recorder) played(t *testing.T) playback.Request {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		r.mu.Lock()
		n := len(r.reqs)
		r.mu.Unlock()
		if n > 0 {
			r.mu.Lock()
			defer r.mu.Unlock()
			req := r.reqs[n-1]
			r.reqs = nil
			return req
		}
	}
	t.Fatal("nothing played")
	return playback.Request{}
}

func recordingApp() (*app, *recorder) {
	a := newTestApp()
	a.settings = defaultSettings()
	rec := &recorder{Fake: playback.NewFake()}
	a.pl = playback.New(func(func()) {}, rec)
	return a, rec
}

// wantRadio fails unless req plays the radio of uri, from the song first
// if set.
func wantRadio(t *testing.T, req playback.Request, uri, first string) {
	t.Helper()
	if req.Context != music.StationURI(uri) {
		t.Fatalf("played %q, want the radio of %s", req.Context, uri)
	}
	if first == "" && len(req.Tracks) > 0 || first != "" && (len(req.Tracks) == 0 || req.Tracks[req.Start].ID != first) {
		t.Fatalf("the radio of %s starts from %+v at %d, want %q", uri, req.Tracks, req.Start, first)
	}
}

// A song's menu starts its radio, from the song.
func TestSongMenuStartsRadio(t *testing.T) {
	a, rec := recordingApp()
	tr := music.Track{ID: "75IO", Name: "KISS", Artists: []music.Artist{{ID: "ar", Name: "CRCK/LCKS"}}, Duration: 219000}
	a.pl.Cue([]music.Track{tr})
	a.setLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	if err := tt.Click("更多"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("开始电台"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	wantRadio(t, rec.played(t), "spotify:track:75IO", "75IO")
}

// An artist's page starts their radio, of songs like theirs.
func TestArtistPageStartsRadio(t *testing.T) {
	a, rec := recordingApp()
	a.artists["ar"] = &artistData{profile: &music.ArtistProfile{
		Artist: music.Artist{ID: "ar", Name: "CRCK/LCKS"},
		Songs:  []music.Track{{ID: "1", Name: "KISS", Duration: 219000}},
	}}
	a.route = route{page: pageArtist, id: "ar"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.Click("电台"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	wantRadio(t, rec.played(t), "spotify:artist:ar", "")
}

// The home page's radios of the listener's top artists and songs play
// those radios, from a click anywhere on their cards.
func TestHomeRadios(t *testing.T) {
	a, rec := recordingApp()
	a.home = music.Home{
		TopArtists: []music.ArtistProfile{{Artist: music.Artist{ID: "ar", Name: "CRCK/LCKS"}}},
		TopTracks:  []music.Track{{ID: "75IO", Name: "KISS", Artists: []music.Artist{{Name: "CRCK/LCKS"}}, Duration: 219000}},
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.Click("CRCK/LCKS 电台"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	wantRadio(t, rec.played(t), "spotify:artist:ar", "")
	if err := tt.Click("KISS 电台"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	wantRadio(t, rec.played(t), "spotify:track:75IO", "75IO")
}
