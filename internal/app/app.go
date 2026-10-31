package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygoify/internal/library"
	"mygoify/internal/music"
	"mygoify/internal/playback"
)

type pageKind int

const (
	pageHome pageKind = iota
	pagePlaylist
	pageSearch
	pageArtist
	pageAlbum
)

type route struct {
	page pageKind
	// id is the playlist, artist or album the page shows.
	id string
	// query and kind are what a search page searched for.
	query string
	kind  music.SearchType
}

// app is the state both windows show. Everything in it is read and changed
// on the main thread: by the views, and by goroutines through post.
type app struct {
	win   *mygo.Window
	notch *notch
	tray  *mygo.Tray // the icon in the menu bar or notification area, while it shows
	art   *artCache
	lib   library.Library
	pl    *playback.Player
	pal   palette

	route   route
	history []route
	forward []route // the pages gone back from, the last first to go to

	profile     *music.User
	mine, saved []music.Playlist // the listener's playlists: made, and saved
	login       loginState
	accountOpen bool            // the dropdown of the account row shows
	naming      nameDialog      // the name of a playlist asked for
	deleting    deleteDialog    // whether to delete a playlist, asked
	liked       map[string]bool // the songs the listener likes
	toast       string          // a message to show once

	home        music.Home
	homeErr     string
	homeLoading bool
	homeAt      time.Time // when what the home page shows was asked for

	playlists map[string]*music.Playlist
	loadingPl map[string]bool
	plErr     map[string]string
	artists   map[string]*artistData
	artistTab int // what an artist's page lists: artistHot, artistLatest or artistAlbums
	albums    map[string]*albumData
	trackList ui.ListState
	// Where the page and the sidebar are scrolled, kept while Now Playing
	// covers them, unbuilt.
	pageScroll ui.ScrollState
	navScroll  ui.ScrollState
	scrolled   route // the page trackList and pageScroll are of

	mineCollapsed, savedCollapsed bool

	query     string
	searchBox ui.Handle
	searches  map[route]*searchData // the results of each search, by its page
	searched  route                 // the search last shown, which the sidebar goes back to

	nowPlaying  bool
	npOpen      bool      // Now Playing as it last showed: open or not
	npChangedAt time.Time // when it last opened or closed
	npPane      int       // paneLyrics, paneQueue or paneStage
	npPaneTop   float32   // how far the cover beside the pane starts below its top
	lyricScroll ui.ScrollState
	lyricGlide  lyricGlide
	lyricAt     int
	lyricTrack  string
	lyr         lyricsState // the lyrics of the track playing
	queueList   ui.ListState
	queueAt     int // the row of the queue pane last scrolled to, -1 while it is hidden

	scrub      float64
	scrubbing  bool
	volume     float64
	volumeOpen bool // the volume popover of the player bar shows
	mutedFrom  float64

	rowBars barState
	// underTop and underBottom say whether the page has content under its
	// bars, which show their scroll edges then.
	underTop, underBottom bool
	barScrub              scrubState
	npScrub               scrubState
	backdropCache         backdropCache // the backdrop of Now Playing, as last painted
	npArt                 *artwork      // the cover whose colors Now Playing shows
	npCoverState          coverState    // the cover of Now Playing, as it is tilted and flicked
	wallState             wallState     // the wall of the queue's covers, as it is dragged
	sig                   playerSig
	quitting              bool

	settings    settings
	settingsWin *mygo.Window
	updates     bool // the update plugin is used (useUpdater)
}

// post runs fn on the main thread and redraws both windows. Without a
// window, as in tests, it drops fn.
func (a *app) post(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		fn()
		a.redraw()
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.notch != nil && a.notch.win != nil {
		a.notch.win.Invalidate()
	}
}

// shotHook, when set, drives the app to check its design.
var shotHook func(a *app)

func (a *app) showMain() {
	if a.win != nil {
		if a.win.IsMinimized() {
			a.win.Restore()
		}
		a.win.Show()
		a.win.Focus()
		// From the menu bar alone the app may be inactive.
		mygo.App.Focus()
	}
}

// Run runs the app until it quits.
func Run() error {
	a := &app{
		playlists: map[string]*music.Playlist{},
		loadingPl: map[string]bool{},
		plErr:     map[string]string{},
		artists:   map[string]*artistData{},
		albums:    map[string]*albumData{},
		searches:  map[route]*searchData{},
		lyricAt:   -1,
		queueAt:   -1,
		// Spotify answers in M2 and M3: until then, made-up music.
		lib: library.NewFake(),
	}
	a.useUpdater()
	mygo.App.WhenReady(func() {
		a.settings = loadSettings()
		a.settings.applyAppearance()
		a.npPane = a.lyricsPaneChosen()
		// Before the window: on Windows it lays out its content as it is
		// made, and the view asks the player what plays.
		a.art = newArtCache(a.post)
		var backend playback.Backend = playback.NewFake()
		if lib, b := connectSpotify(a.settings.Spotify, secrets()); lib != nil {
			a.lib, backend = lib, b
		}
		a.pl = playback.New(a.post, backend)
		a.pl.Artwork = fetchArt
		a.pl.Changed = a.playbackChanged
		// While Spotify limits the app's calls, until when.
		a.pl.Explain = failed
		a.volume = a.settings.Volume
		a.pl.SetVolume(a.volume)
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         mygo.App.Name(),
			Width:         1180,
			Height:        780,
			MinWidth:      680,
			MinHeight:     580,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHidden,
			// As tall as the edge over a page (Linux, Windows).
			TitleBarHeight: buttonsHeight,
			// Inset from the corner, within the sidebar, as macOS 26's
			// apps with a sidebar place them.
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			Vibrancy:             mygo.VibrancySidebar,
			BackgroundColor:      "light-dark(#ffffff, #141416)",
			Content:              ui.View(a.view),
		})
		// Now that post brings what the backend says to the window.
		a.pl.Follow()
		a.setMenu()
		a.boot()
		// The notch player is macOS's, over its menu bar: elsewhere it would
		// hang at the top of the screen as a black box.
		if runtime.GOOS == "darwin" {
			a.notch = newNotch(a)
		}
		// From the menu bar alone, the Dock icon shows while the window
		// does.
		a.syncDock()
		a.win.OnShow(a.syncDock)
		a.win.OnHide(a.syncDock)
		// Shown or switched to the morning after, it shows the new day's.
		a.win.OnFocus(a.freshenHome)
		a.applyTray()
		if shotHook != nil {
			shotHook(a)
		}
		// Closing the window hides it: music plays on, and the Dock icon
		// or the tray's brings it back. A Linux desktop without a tray
		// has neither, so there closing it quits.
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.quitting {
				return
			}
			e.PreventDefault()
			if runtime.GOOS == "darwin" || a.tray != nil {
				a.win.Hide()
			} else {
				mygo.App.Quit()
			}
		})
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.settings.Volume = a.volume
		a.settings.save()
	})
	// Windows leaves an icon in the notification area after its app until
	// the pointer passes over it.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { a.showMain() })
	return mygo.App.Run()
}
