package app

import (
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

// With NM_SHOTS set to a directory, the app walks through its views and
// saves what each window shows there, for checking the design. It runs on
// whatever library the app has: the fake one, until Spotify answers.
func init() {
	dir := os.Getenv("NM_SHOTS")
	if dir == "" {
		return
	}
	shotHook = func(a *app) {
		// The walk changes settings: keep the listener's own as they were.
		a.settings.path = ""
		go func() {
			shot := func(name string, w *mygo.Window) {
				if w == nil {
					return
				}
				png, err := w.CapturePage()
				if err == nil {
					os.WriteFile(filepath.Join(dir, name+".png"), png, 0o644)
				}
			}
			step := func(d time.Duration, fn func()) {
				time.Sleep(d)
				mygo.RunOnMain(func() { fn(); a.redraw() })
			}
			// waitFor waits up to ten seconds for ok, on the main thread.
			waitFor := func(ok func() bool) {
				for range 40 {
					var done bool
					mygo.RunOnMain(func() { done = ok() })
					if done {
						return
					}
					time.Sleep(250 * time.Millisecond)
				}
			}
			notch := func() *mygo.Window {
				if a.notch == nil {
					return nil
				}
				return a.notch.win
			}
			mygo.RunOnMain(func() { a.win.Show(); a.win.Focus() })

			waitFor(func() bool { return len(a.home.Recent) > 0 && len(a.mine) > 1 })
			time.Sleep(time.Second) // the covers
			shot("1-home", a.win)
			shot("1-notch-idle", notch())

			var playlist string
			step(0, func() {
				playlist = a.mine[1].ID // the first after the liked songs
				a.open(route{page: pagePlaylist, id: playlist})
			})
			waitFor(func() bool { return a.playlists[playlist] != nil })
			time.Sleep(time.Second)
			shot("2-playlist", a.win)

			step(0, func() {
				pl := a.playlists[playlist]
				a.volume = 0
				a.pl.SetVolume(0)
				a.pl.PlayQueue(pl.URI(), pl.Tracks, firstPlayable(pl.Tracks))
			})
			time.Sleep(2 * time.Second)
			shot("3-playing", a.win)
			shot("3-notch-compact", notch())
			if a.notch != nil {
				step(0, func() { a.notch.open = true; a.notch.lastInside = time.Now().Add(time.Hour) })
				time.Sleep(2 * time.Second)
				shot("4-notch-open", notch())
				step(0, func() { a.notch.open = false })
			}

			var album, artist string
			step(0, func() {
				tr := a.pl.Current()
				album, artist = tr.Album.ID, tr.Artists[0].ID
				a.open(route{page: pageAlbum, id: album})
			})
			waitFor(func() bool { return a.albums[album] != nil && a.albums[album].album != nil })
			time.Sleep(time.Second)
			shot("5-album", a.win)
			step(0, func() { a.open(route{page: pageArtist, id: artist}) })
			waitFor(func() bool { return a.artists[artist] != nil && a.artists[artist].profile != nil })
			time.Sleep(time.Second)
			shot("6-artist", a.win)

			step(0, func() { a.search("夜") })
			waitFor(func() bool { d := a.searches[a.route]; return d != nil && d.page.loaded })
			time.Sleep(time.Second)
			shot("7-search", a.win)

			for _, p := range []struct {
				name string
				pane int
			}{{"8-lyrics", paneLyrics}, {"9-stage", paneStage}, {"10-queue", paneQueue}, {"11-wall", paneWall}} {
				step(0, func() { a.nowPlaying, a.npPane = true, p.pane })
				time.Sleep(3 * time.Second)
				shot(p.name, a.win)
			}
			mygo.RunOnMain(mygo.App.Quit)
		}()
	}
}
