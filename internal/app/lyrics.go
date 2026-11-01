package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mygoify/internal/lyrics"
	"mygoify/internal/music"
)

// lyricsState is what the app has of the lyrics of the track playing.
type lyricsState struct {
	track  string // the track they are of
	lines  []lyrics.Line
	loaded bool
}

// fetchLyrics finds a track's timed lyrics, or none, on the lyrics server
// chosen in the settings; tests replace it. Made-up music has none:
// Spotify's, from LRCLIB (useLRCLIB).
var fetchLyrics = func(tr music.Track, server string) ([]lyrics.Line, error) { return nil, nil }

// lyricsLater runs f after d, off the main thread; tests run it by hand.
var lyricsLater = func(d time.Duration, f func()) { time.AfterFunc(d, f) }

// lyricsWait is how long lyrics show as loading before they show as none:
// LRCLIB may take a quarter of a minute to say it is busy (DESIGN.md §7),
// and lyrics found later show then.
const lyricsWait = 4 * time.Second

// lyricsRetries are how long the app waits, one after another, to ask
// again for the lyrics of the track playing that LRCLIB couldn't be asked
// for.
var lyricsRetries = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute}

// useLRCLIB has fetchLyrics ask LRCLIB, keeping what it finds in the
// cache directory.
func useLRCLIB() {
	client := &http.Client{Timeout: 20 * time.Second}
	var cache string
	if c, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(c, "mygoify", "lyrics")
	}
	fetchLyrics = func(tr music.Track, server string) ([]lyrics.Line, error) {
		l := &lyrics.LRCLIB{HTTP: client, Base: strings.TrimRight(strings.TrimSpace(server), "/"), Cache: cache}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		q := lyrics.Query{ID: tr.ID, Name: tr.Name, Album: tr.Album.Name, Duration: time.Duration(tr.Duration) * time.Millisecond}
		if len(tr.Artists) > 0 {
			q.Artist = tr.Artists[0].Name
		}
		return l.Find(ctx, q)
	}
}

// currentLyrics returns the lyrics of the track playing, and whether they
// have loaded.
func (a *app) currentLyrics() ([]lyrics.Line, bool) {
	tr := a.pl.Current()
	if tr == nil || a.lyr.track != tr.ID {
		return nil, false
	}
	return a.lyr.lines, a.lyr.loaded
}

// playbackChanged hears that what plays, or how, changed: a new track's
// lyrics load, and the next one's, to be at hand as it plays.
func (a *app) playbackChanged() {
	tr := a.pl.Current()
	if tr == nil || tr.ID == a.lyr.track {
		return
	}
	a.lyr = lyricsState{track: tr.ID}
	a.askLyrics(*tr, 0)
	id := tr.ID
	lyricsLater(lyricsWait, func() {
		a.post(func() {
			if a.lyr.track == id {
				a.lyr.loaded = true
			}
		})
	})
	if order, at := a.pl.PlayOrder(); at >= 0 && at+1 < len(order) {
		a.askLyrics(a.pl.Queue()[order[at+1]], 0)
	}
}

// askLyrics fetches t's lyrics, unless they are being fetched already, and
// shows them if t plays as they come. Of a track playing that LRCLIB
// couldn't be asked for, it asks again later, the retry'th time.
func (a *app) askLyrics(t music.Track, retry int) {
	if a.lyrAsking[t.ID] {
		return
	}
	if a.lyrAsking == nil {
		a.lyrAsking = map[string]bool{}
	}
	a.lyrAsking[t.ID] = true
	server := a.settings.LyricsServer
	go func() {
		lines, err := fetchLyrics(t, server)
		a.post(func() {
			delete(a.lyrAsking, t.ID)
			if err != nil {
				log.Printf("lyrics of %s: %v", t.ID, err)
			}
			// Of another track, they were fetched for the cache.
			if a.lyr.track != t.ID {
				return
			}
			// Unfound shows as none, not as loading still.
			a.lyr.loaded = true
			if err == nil {
				a.lyr.lines = lines
				return
			}
			if retry < len(lyricsRetries) {
				lyricsLater(lyricsRetries[retry], func() {
					a.post(func() {
						if a.lyr.track == t.ID && len(a.lyr.lines) == 0 {
							a.askLyrics(t, retry+1)
						}
					})
				})
			}
		})
	}()
}

// setLyrics gives the track playing lines, as loaded, for tests.
func (a *app) setLyrics(lines []lyrics.Line) {
	if tr := a.pl.Current(); tr != nil {
		a.lyr = lyricsState{track: tr.ID, lines: lines, loaded: true}
	}
}
