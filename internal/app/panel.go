package app

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygoify/internal/library"
	"mygoify/internal/music"
	"mygoify/internal/omarchy"
	"mygoify/internal/playback"
	"mygoify/locales"
)

// The mini player is a process of its own, started as its icon is clicked
// and gone as it closes, so that it costs nothing while it doesn't show
// (DESIGN.md §4.3). It follows the Soloist that plays, and asks Spotify for
// likes and saves as the app does; a link in it opens the app on its page.

// miniTitle is the mini player's window's title, which Hyprland's rule
// for it matches: it is never shown.
const miniTitle = "MyGoify Mini Player"

// The mini player's size: as wide as Omarchy's own drop-downs.
const (
	miniWidth  = 340
	miniHeight = 600
)

// miniSocket is where an open mini player listens: another started closes
// it, as a second click on the icon does.
func miniSocket() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "mygoify-mini.sock")
}

// handOver closes the mini player that is open, and reports whether there
// was one.
func handOver() bool {
	c, err := net.DialTimeout("unix", miniSocket(), time.Second)
	if err != nil {
		return false
	}
	c.Write([]byte("close\n"))
	c.Close()
	return true
}

// RunPanel runs the mini player until it closes, or closes the one open.
func RunPanel() error {
	if handOver() {
		return nil
	}
	os.Remove(miniSocket())
	ln, err := net.Listen("unix", miniSocket())
	if err != nil {
		return err
	}
	defer os.Remove(miniSocket())
	a := &app{
		playlists: map[string]*music.Playlist{}, loadingPl: map[string]bool{}, plErr: map[string]string{},
		artists: map[string]*artistData{}, albums: map[string]*albumData{}, searches: map[route]*searchData{},
		lyricAt: -1, queueAt: -1,
		lib:  library.NewFake(),
		mini: true,
	}
	var spot omarchy.Spot
	var placed bool
	mygo.App.WhenReady(func() {
		a.settings = loadSettings()
		if a.settings.Language != "" {
			mygo.App.SetLocale(a.settings.Language)
		}
		a.settings.applyAppearance()
		a.art = newArtCache(a.post)
		a.keep = secrets(a.settings.Spotify.SecretsInFiles)
		var backend playback.Backend = playback.NewFake()
		if lib, b, _ := connect(a.settings.Spotify, a.keep, false); lib != nil {
			a.lib, backend, a.live = lib, b, true
		}
		a.pl = playback.New(a.post, backend)
		// The agent shows the system what plays.
		a.pl.Quiet = true
		a.volume = keptVolume()
		h := miniHeight
		if omarchy.Hyprland() {
			if spot, placed = omarchy.PanelAt(miniWidth); placed {
				h = min(h, spot.MaxHeight)
				if err := omarchy.PlacePanel(miniTitle, spot); err != nil {
					log.Print(err)
				}
			}
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:             miniTitle,
			Width:             miniWidth,
			Height:            h,
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: true,
			DisableResize:     true,
			DisableMinimize:   true,
			DisableMaximize:   true,
			DisableFullScreen: true,
			SkipTaskbar:       true,
			AlwaysOnTop:       true,
			Content:           ui.View(a.miniView),
		})
		a.pl.Follow()
		go func() {
			prof, _ := a.lib.Me()
			a.post(func() { a.profile = prof })
		}()
		// Another started closes this one: the icon clicked again.
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Close()
				a.post(a.closeMini)
			}
		}()
		switch {
		case omarchy.Hyprland() && placed:
			// A click outside closes it, as Omarchy's drop-downs do.
			a.stopClicks = omarchy.WatchClicks(func() {
				x, y, ok := omarchy.Cursor()
				if ok && (x < spot.Left || x >= spot.Left+miniWidth || y < spot.Top || y >= spot.Top+h) {
					a.post(a.closeMini)
				}
			})
		default:
			// Elsewhere, as another window takes the focus.
			a.win.OnBlur(a.closeMini)
		}
		a.win.OnClose(func(*mygo.CloseEvent) { a.closeMini() })
	})
	return mygo.App.Run()
}

// closeMini closes the mini player, and its process with it.
func (a *app) closeMini() {
	if a.stopClicks != nil {
		a.stopClicks()
		a.stopClicks = nil
	}
	quitApp()
}

// keptVolume is the volume Soloist plays at, as the app keeps it beside
// its data, from 0 to 1.
func keptVolume() float64 {
	b, err := os.ReadFile(filepath.Join(dataDir(), "soloist.volume"))
	if err != nil {
		return 0.4
	}
	_, v, _ := strings.Cut(strings.TrimSpace(string(b)), "=")
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0.4
	}
	return float64(max(0, min(100, n))) / 100
}

// miniPalette is the mini player's colors: on Omarchy its theme's, as its
// own panels are drawn, else the app's.
func miniPalette(t *ui.Theme) (palette, omarchy.Theme, bool) {
	th, ok := omarchy.Theme{}, false
	if omarchy.Detect() {
		th, ok = omarchy.Current()
	}
	if !ok {
		return paletteFor(t), th, false
	}
	p := palette{
		dark: th.Dark, bg: th.Background, sidebar: th.Background,
		text: th.Text, muted: th.Muted, faint: th.Faint, hairline: th.Line,
		hover: th.Hover.Alpha(1), press: th.Press, accent: th.Accent, onAccent: th.OnAccent,
		danger: ui.Hex("#ff453a"), cardShade: ui.RGBA(0, 0, 0, 0.3),
	}
	return p, th, true
}

// The mini player is laid out as Omarchy's own panels are: square, a
// hero row over sections of small capitals, hairline sliders, and rows
// that fill edge to edge under the pointer.
const (
	miniPad   = 14 // its inset from its frame
	miniRow   = 40 // a row of what plays next
	miniIconB = 26 // a square icon button
)

// miniView is the mini player: what plays, with its controls, and what
// plays next, which a click plays.
func (a *app) miniView(c *ui.Context) {
	t := c.Theme()
	pal, th, om := miniPalette(t)
	a.pal = pal
	p := a.pal
	root := ui.Column(c).Fill().Background(p.bg).TextColor(p.text).FontSize(12.5)
	if om {
		// Framed as Hyprland frames windows.
		if th.BorderWidth > 0 {
			root.Border(float32(th.BorderWidth), th.Border)
		}
		if th.Font != "" {
			root.Font(th.Font + ", system-ui")
		}
		root.Radius(float32(th.Rounding))
	} else {
		root.Radius(10)
	}
	c.OnShortcut(0, ui.KeyEscape, a.closeMini)
	a.npScrub.thin = true
	root.Children(func() {
		tr := a.pl.Current()
		ui.Column(c).Padding(miniPad).Gap(14).Children(func() {
			a.miniHero(c, tr)
			if tr == nil {
				return
			}
			// Where it is, first: the bar and its times under it.
			ui.Column(c).Gap(2).Children(func() {
				a.scrubber(c.Key("mini-scrub"), &a.npScrub, p.hairline, p.text, 14, ui.Transparent)
				ui.Row(c).Children(func() {
					ui.Text(c, clock(a.pl.Position())).FontSize(10.5).FontFeatures("tnum").TextColor(p.muted)
					ui.Spacer(c)
					ui.Text(c, "-"+clock(max(0, a.pl.Duration()-a.pl.Position()))).FontSize(10.5).FontFeatures("tnum").TextColor(p.muted)
				})
			})
			a.transport(c, p.text, p.faint, p.text, 18)
			// The volume, after: faint until the pointer comes to it.
			a.miniVolume(c)
		})
		if tr == nil {
			return
		}
		ui.Box(c).Height(1).Background(p.hairline).MarginX(miniPad)
		a.miniQueue(c)
	})
}

// miniHero is the row at the top, as Omarchy's panels open with: the
// cover, which opens its album in the app, what plays and by whom, each
// opening its page, and the buttons of the heart, the album's save and
// the app.
func (a *app) miniHero(c *ui.Context, tr *music.Track) {
	p := a.pal
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		if tr == nil {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "MyGoify").FontSize(14).FontWeight(700)
				miniCaption(c, locales.NotPlaying(), p.muted)
			})
			a.miniOpenApp(c)
			return
		}
		a.wantLike(tr.ID)
		cov := ui.ButtonBase(c.Key("cover")).FocusRing(false).Label(locales.AlbumCover())
		cov.Children(func() { a.cover(c, tr.Album.Images, 160, 52, 0) })
		if cov.Clicked() && tr.Album.ID != "" {
			a.openPage(route{page: pageAlbum, id: tr.Album.ID})
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.RichText(c).FontSize(14).FontWeight(700).Children(func() {
				if tr.Album.ID != "" {
					a.link(c.Key("name"), tr.Name, p.text, p.accent, route{page: pageAlbum, id: tr.Album.ID})
				} else {
					ui.Text(c, tr.Name)
				}
			}).SingleLine()
			a.artistLinks(c, tr, p.muted, p.text).FontSize(12).SingleLine()
			if tr.Album.Name != "" {
				miniCaption(c, tr.Album.Name, p.faint)
			}
		})
		ui.Row(c).Gap(2).AlignSelf(ui.Start).Children(func() {
			a.miniLike(c, tr)
			a.miniSaveAlbum(c, tr)
			a.miniOpenApp(c)
		})
	})
}

// miniCaption is a line of small capitals, as Omarchy sets its captions
// and the heads of its sections.
func miniCaption(c *ui.Context, text string, color ui.Color) ui.Element {
	return ui.Text(c, strings.ToUpper(text)).FontSize(10).FontWeight(700).LetterSpacing(1.2).TextColor(color).SingleLine()
}

// miniSection heads a part of the mini player, with a value at its end.
func (a *app) miniSection(c *ui.Context, title, value string) {
	ui.Row(c).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
		miniCaption(c, title, a.pal.muted)
		ui.Spacer(c)
		ui.Text(c, value).FontSize(10).FontWeight(700).FontFeatures("tnum").TextColor(a.pal.muted)
	})
}

// miniButton is a square icon button, filled under the pointer as
// Omarchy's are.
func (a *app) miniButton(c *ui.Context, icon *ui.SVG, color ui.Color, label string) bool {
	b := ui.ButtonBase(c).Size(miniIconB, miniIconB).Center().FocusRing(false).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(a.pal.hover)
	}
	b.Children(func() { ui.Icon(c, icon).Size(15, 15).TextColor(color) })
	return b.Clicked()
}

// miniLike is a track's heart, as the mini player's other buttons are.
func (a *app) miniLike(c *ui.Context, tr *music.Track) {
	if a.profile == nil {
		return
	}
	icon, color, label := iconHeart, a.pal.muted, locales.Like()
	if a.isLiked(tr) {
		icon, color, label = iconHeartFill, a.pal.accent, locales.Unlike()
	}
	if a.miniButton(c.Key("like-"+tr.ID), icon, color, label) {
		a.toggleLike(tr)
	}
}

// miniOpenApp opens the app's window, and closes the mini player.
func (a *app) miniOpenApp(c *ui.Context) {
	if a.miniButton(c.Key("open-app"), iconOpen, a.pal.muted, locales.OpenMyGoify()) {
		openApp()
		a.closeMini()
	}
}

// miniSaveAlbum is the button that saves the album playing, once its page
// is read, which says whether it is saved.
func (a *app) miniSaveAlbum(c *ui.Context, tr *music.Track) {
	if tr.Album.ID == "" || a.profile == nil {
		return
	}
	d := a.albums[tr.Album.ID]
	if d == nil {
		a.loadAlbum(tr.Album.ID)
		return
	}
	if d.album == nil {
		return
	}
	icon, label, color := iconPlus, locales.SaveAlbum(), a.pal.muted
	if d.album.Saved {
		icon, label, color = iconCheck, locales.AlbumSaved(), a.pal.accent
	}
	if a.miniButton(c.Key("save-album"), icon, color, label) {
		a.toggleAlbumSaved(tr.Album.ID)
	}
}

// miniVolume is the volume: a speaker and a hairline, faint beside where
// the track is, as it matters less, and brightening with its knob as the
// pointer comes to it.
func (a *app) miniVolume(c *ui.Context) {
	p := a.pal
	row := ui.Row(c.Key("volume-row")).Gap(8).AlignItems(ui.Center).Padding(0, 40)
	hot := row.Hovered()
	row.Children(func() {
		icon := iconVolume
		switch {
		case a.volume == 0:
			icon = iconMute
		case a.volume < 0.5:
			icon = iconVolumeLow
		}
		ui.Icon(c, icon).Size(13, 13).TextColor(p.faint)
		s := ui.SliderBase(c.Key("volume"), &a.volume, 0, 1).Grow(1).Height(14).FocusRing(false).Label(locales.Volume()).Cursor(ui.CursorPointer)
		if s.Changed() {
			a.pl.SetVolume(a.volume)
		}
		lit := float32(0)
		if hot || s.Pressed() {
			lit = 1
		}
		lit = s.AnimateTo("lit", lit, ui.Snappy)
		v := float32(a.volume)
		fill := p.faint.Mix(p.text, lit)
		s.Draw(func(pt *ui.Painter, r ui.Rect) {
			y := r.Y + r.H/2
			pt.Fill(ui.Rect{X: r.X, Y: y - 1, W: r.W, H: 2}, p.hairline, 1)
			pt.Fill(ui.Rect{X: r.X, Y: y - 1, W: r.W * v, H: 2}, fill, 1)
			if lit > 0.01 {
				k := 10 * lit
				pt.Fill(ui.Rect{X: r.X + r.W*v - k/2, Y: y - k/2, W: k, H: k}, fill, k/2)
			}
		})
	})
}

// miniQueue lists what plays next, a click playing it.
func (a *app) miniQueue(c *ui.Context) {
	p := a.pal
	pl := a.pl
	order, at := pl.PlayOrder()
	var next []int
	if at >= 0 && at+1 < len(order) {
		next = order[at+1:]
	}
	ui.Box(c).Padding(14, miniPad, 6, miniPad).Children(func() {
		a.miniSection(c, locales.UpNext(), strconv.Itoa(len(next)))
	})
	if len(next) == 0 {
		ui.Box(c).Padding(8, miniPad, 14, miniPad).Children(func() {
			ui.Text(c, locales.NothingUpNext()).TextColor(p.faint)
		})
		return
	}
	a.queueList.Key = func(row int) any { return next[row] }
	ui.List(c.Key("mini-queue"), &a.queueList, len(next), func(r int) {
		i := next[r]
		tr := &pl.Queue()[i]
		a.wantLike(tr.ID)
		row := ui.ButtonBase(c.Key(i)).Height(miniRow).PaddingX(miniPad).Gap(10).AlignItems(ui.Center).FocusRing(false).Label(tr.Name).
			ContextMenu(a.songItems(tr, ""))
		hovered := row.Hovered()
		if hovered {
			row.Background(p.hover)
		}
		if !tr.Playable() {
			row.Opacity(0.4)
		}
		row.Children(func() {
			a.cover(c, tr.Album.Images, 96, 28, 0)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, tr.Name).FontWeight(600).SingleLine()
				a.artistLinks(c, tr, p.muted, p.text).FontSize(11).SingleLine()
			})
			if hovered || a.isLiked(tr) {
				a.miniLike(c, tr)
			}
		})
		if row.Clicked() && tr.Playable() {
			pl.Jump(i)
		}
	}).Grow(1).Padding(0, 0, 6, 0)
}
