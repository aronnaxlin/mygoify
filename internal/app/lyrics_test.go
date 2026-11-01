package app

import (
	"errors"
	"testing"
	"time"

	"mygoify/internal/lyrics"
	"mygoify/internal/music"
)

// A lyricsAsk is the app asking for a track's lyrics, waiting for reply.
type lyricsAsk struct {
	id    string
	src   lyricsSources
	reply chan lyricsReply
}

type lyricsReply struct {
	lines []lyrics.Line
	err   error
}

// lyricsRig is a test app whose lyrics are asked for on asks, what it
// posts kept in posted to be run by hand, and what it would run later in
// later.
type lyricsRig struct {
	t      *testing.T
	a      *app
	asks   chan lyricsAsk
	posted chan func()
	later  []func()
}

func newLyricsRig(t *testing.T) *lyricsRig {
	r := &lyricsRig{t: t, a: newTestApp(), asks: make(chan lyricsAsk, 4), posted: make(chan func(), 4)}
	r.a.inline = func(f func()) { r.posted <- f }
	fetch, later := fetchLyrics, lyricsLater
	t.Cleanup(func() { fetchLyrics, lyricsLater = fetch, later })
	fetchLyrics = func(tr music.Track, src lyricsSources) ([]lyrics.Line, error) {
		reply := make(chan lyricsReply)
		r.asks <- lyricsAsk{tr.ID, src, reply}
		v := <-reply
		return v.lines, v.err
	}
	lyricsLater = func(_ time.Duration, f func()) { r.later = append(r.later, f) }
	return r
}

// ask waits for the app to ask for lyrics.
func (r *lyricsRig) ask() lyricsAsk {
	r.t.Helper()
	select {
	case q := <-r.asks:
		return q
	case <-time.After(5 * time.Second):
		r.t.Fatal("lyrics not asked for")
		return lyricsAsk{}
	}
}

// run runs what the app posts next.
func (r *lyricsRig) run() {
	r.t.Helper()
	select {
	case f := <-r.posted:
		f()
	case <-time.After(5 * time.Second):
		r.t.Fatal("nothing posted")
	}
}

// answer replies to q, running what the app posts of it.
func (r *lyricsRig) answer(q lyricsAsk, lines []lyrics.Line, err error) {
	r.t.Helper()
	q.reply <- lyricsReply{lines, err}
	r.run()
}

// play makes the tracks of ids the queue, the first playing.
func (r *lyricsRig) play(ids ...string) {
	var tracks []music.Track
	for _, id := range ids {
		tracks = append(tracks, music.Track{ID: id})
	}
	r.a.pl.Cue(tracks)
	r.a.playbackChanged()
}

func (r *lyricsRig) shows(lines int, loaded bool) {
	r.t.Helper()
	if l, ok := r.a.currentLyrics(); len(l) != lines || ok != loaded {
		r.t.Fatalf("%d lines, loaded %v; want %d, %v", len(l), ok, lines, loaded)
	}
}

var oneLine = []lyrics.Line{{Text: "春日影"}}

// Lyrics LRCLIB couldn't be asked for show as none, not as loading: the
// panel shimmered on for good.
func TestLyricsUnfoundShowNone(t *testing.T) {
	r := newLyricsRig(t)
	r.play("1")
	r.answer(r.ask(), nil, errors.New("LRCLIB: 503"))
	r.shows(0, true)
}

// Lyrics LRCLIB is slow to find show as none after a while, and then as
// they come.
func TestLyricsWaitCapped(t *testing.T) {
	r := newLyricsRig(t)
	r.play("1")
	q := r.ask()
	r.shows(0, false)
	r.later[0]()
	r.run()
	r.shows(0, true)
	r.answer(q, oneLine, nil)
	r.shows(1, true)
}

// Lyrics of the track playing that no source could be asked for are asked
// for again later, of the sources chosen, and not once another plays.
func TestLyricsAskedAgain(t *testing.T) {
	r := newLyricsRig(t)
	r.a.settings.LyricsServer = "https://mirror.example"
	r.a.settings.LyricsNetEase = true
	r.a.settings.LyricsQQ = true
	r.play("1")
	r.answer(r.ask(), nil, errors.New("LRCLIB: 503"))
	again := r.later[len(r.later)-1]
	again()
	r.run()
	q := r.ask()
	if q.id != "1" || q.src.Server != "https://mirror.example" || !q.src.NetEase || !q.src.QQ {
		t.Fatalf("asked for %s of %+v", q.id, q.src)
	}
	r.answer(q, nil, errors.New("LRCLIB: 503"))
	again = r.later[len(r.later)-1]

	r.play("2")
	r.ask()
	again()
	r.run()
	if r.a.lyrAsking["1"] {
		t.Fatal("asked again for a track no longer playing")
	}
}

// The next track's lyrics are fetched as one plays, and show as they come
// once it plays, without being asked for twice.
func TestLyricsOfNextFetched(t *testing.T) {
	r := newLyricsRig(t)
	r.play("1", "2")
	asks := map[string]lyricsAsk{}
	for range 2 {
		q := r.ask()
		asks[q.id] = q
	}
	r.answer(asks["1"], nil, nil)

	r.a.pl.Next()
	r.a.playbackChanged()
	r.answer(asks["2"], oneLine, nil)
	r.shows(1, true)
	// A second ask would come from its own goroutine: wait a little for it.
	select {
	case q := <-r.asks:
		t.Fatalf("asked again for %s", q.id)
	case <-time.After(200 * time.Millisecond):
	}
}

// Turning a lyrics source on asks again for the lyrics of the track
// playing, once none show: the source turned on may have them.
func TestLyricsAskedAgainWhenNoLyrics(t *testing.T) {
	r := newLyricsRig(t)
	r.play("1")
	r.answer(r.ask(), nil, nil)
	r.shows(0, true)
	r.a.askLyricsIfNone()
	q := r.ask()
	if q.id != "1" {
		t.Fatalf("asked for %s", q.id)
	}
	r.answer(q, oneLine, nil)
	r.shows(1, true)
	r.a.askLyricsIfNone()
	select {
	case q := <-r.asks:
		t.Fatalf("asked again for %s, which has lyrics", q.id)
	case <-time.After(200 * time.Millisecond):
	}
}
