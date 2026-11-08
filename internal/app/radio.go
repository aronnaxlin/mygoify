package app

import (
	"math/rand/v2"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/library"
	"mygoify/internal/music"
	"mygoify/locales"
)

// startRadio plays Spotify's radio of what uri names: songs like it,
// without end. A song's radio starts with the song, first (DESIGN.md
// §5.4).
func (a *app) startRadio(uri string, first *music.Track) {
	var tracks []music.Track
	if first != nil {
		tracks = []music.Track{*first}
	}
	a.pl.PlayQueue(music.StationURI(uri), tracks, 0)
}

// radioSeeds is how many of the listener's top artists, and then of their
// top songs, the home page offers radios of.
const radioSeeds = 3

// radioCards shows the radios of the listener's top artists and songs, as
// many as fit, taking turns: songs like those they listen to most, as
// Spotify picks them. A click anywhere on one plays it.
func (a *app) radioCards(c *ui.Context, h *music.Home) {
	type seed struct {
		key, name, sub string
		cover          music.Images
		play           func()
	}
	var seeds []seed
	for i := range radioSeeds {
		if i < len(h.TopArtists) {
			ar := &h.TopArtists[i]
			seeds = append(seeds, seed{ar.URI(), ar.Name, locales.Artist(), ar.Images, func() { a.startRadio(ar.URI(), nil) }})
		}
		if i < len(h.TopTracks) {
			tr := &h.TopTracks[i]
			seeds = append(seeds, seed{tr.URI(), tr.Name, tr.ArtistNames(), tr.Album.Images, func() { a.startRadio(tr.URI(), tr) }})
		}
	}
	cols := a.columns(c, 168)
	ui.Grid(c).Columns(cols).GapX(20).GapY(28).Children(func() {
		for _, s := range seeds[:min(len(seeds), cols)] {
			if a.coverCard(c.Key(s.key), locales.RadioOf(s.name), s.cover, s.sub, s.play) {
				s.play()
			}
		}
	})
}

// fakeStation makes up the songs of a radio from lib's, in a random order,
// for the made-up music the app plays until Spotify answers.
func fakeStation(lib library.Library) func(string) []music.Track {
	return func(string) []music.Track {
		res, err := lib.Search("", music.SearchTracks, 20, 0)
		if err != nil {
			return nil
		}
		rand.Shuffle(len(res.Tracks), func(i, j int) { res.Tracks[i], res.Tracks[j] = res.Tracks[j], res.Tracks[i] })
		return res.Tracks
	}
}
