package app

import (
	"mygoify/internal/lyrics"
	"mygoify/internal/music"
)

// lyricsState is what the app has of the lyrics of the track playing.
type lyricsState struct {
	track  string // the track they are of
	lines  []lyrics.Line
	loaded bool
}

// fetchLyrics finds a track's timed lyrics, or none; tests replace it.
// LRCLIB answers in M5: until then there are none.
var fetchLyrics = func(tr music.Track) ([]lyrics.Line, error) { return nil, nil }

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
// lyrics load.
func (a *app) playbackChanged() {
	tr := a.pl.Current()
	if tr == nil || tr.ID == a.lyr.track {
		return
	}
	a.lyr = lyricsState{track: tr.ID}
	t := *tr
	go func() {
		lines, err := fetchLyrics(t)
		a.post(func() {
			if a.lyr.track == t.ID {
				a.lyr.lines, a.lyr.loaded = lines, err == nil
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
