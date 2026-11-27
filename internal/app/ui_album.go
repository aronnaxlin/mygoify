package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
	"mygoify/locales"
)

// albumData is what the app has of an album.
type albumData struct {
	album   *music.AlbumProfile
	loading bool
	err     string
	saving  bool // a save or its undoing is on its way
}

func (a *app) loadAlbum(id string) {
	d := a.albums[id]
	if d == nil {
		d = &albumData{}
		a.albums[id] = d
	}
	if d.album != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		al, err := a.lib.Album(id)
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err = failed(err, locales.AlbumLoadFailed())
				return
			}
			d.album = al
		})
	}()
}

func (a *app) albumPage(c *ui.Context) {
	id := a.route.id
	d := a.albums[id]
	if d.album == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadAlbum(id) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, false, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	a.trackRows(c, d.album.Songs, false, func(c *ui.Context) { a.albumHeader(c, d.album) }, func(c *ui.Context) { a.albumFooter(c, d.album) })
}

// albumKind names what an album is: an album, a single or a compilation.
func albumKind(al *music.AlbumProfile) string {
	switch al.Kind {
	case music.SingleKind:
		return locales.Single()
	case music.CompilationKind:
		return locales.Compilation()
	}
	return locales.Album()
}

// albumLength says how long an album plays, to the minute: "1 小时 2 分钟".
func albumLength(d time.Duration) string {
	m := int((d + 30*time.Second) / time.Minute)
	if m < 60 {
		return locales.LengthMinutes(fmt.Sprint(m))
	}
	return locales.LengthHoursMinutes(fmt.Sprint(m/60), fmt.Sprint(m%60))
}

// albumFooter ends an album's songs with when it came out and its
// copyrights, as small print.
func (a *app) albumFooter(c *ui.Context, al *music.AlbumProfile) {
	ui.Column(c).PaddingX(12).Padding(24, 12, 8, 12).Gap(3).Children(func() {
		if al.Released != "" {
			ui.Text(c, al.Released).FontSize(12.5).FontFeatures("tnum").TextColor(a.pal.muted).Margin(0, 0, 4, 0)
		}
		for _, cr := range al.Copyrights {
			ui.Text(c, cr).FontSize(11.5).TextColor(a.pal.faint)
		}
	})
}

// toggleAlbumSaved saves an album to the listener's library, or takes it
// out, at once, going back if Spotify refuses. Signed out, it asks the
// listener to sign in.
func (a *app) toggleAlbumSaved(id string) {
	d := a.albums[id]
	if d == nil || d.album == nil || d.saving {
		return
	}
	if a.profile == nil {
		a.openOnboarding()
		return
	}
	al := d.album
	save := !al.Saved
	al.Saved, d.saving = save, true
	go func() {
		err := a.lib.SaveAlbum(id, save)
		a.post(func() {
			d.saving = false
			switch {
			case err != nil:
				al.Saved = !save
				a.toast = locales.ActionFailed()
				return
			case save:
				a.toast = locales.SavedToLibrary()
			default:
				a.toast = locales.RemovedFromLibrary()
			}
			// Their page lists it, or doesn't, as it loads anew.
			a.savedAlbums, a.savedAlbumsPage = nil, paged{}
		})
	}()
}

func (a *app) albumHeader(c *ui.Context, al *music.AlbumProfile) {
	p := a.pal
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		ui.Box(c).Size(headerArt(c), headerArt(c)).Radius(14).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, al.Images, 600, 14)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, albumKind(al)).FontSize(12).FontWeight(600).TextColor(p.muted)
			ui.Text(c, al.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			ui.RichText(c).FontSize(13).TextColor(p.muted).Children(func() {
				for i, ar := range al.Artists {
					if i > 0 {
						ui.Text(c, ", ")
					}
					a.link(c.Key(i), ar.Name, p.muted, p.text, route{page: pageArtist, id: ar.ID})
				}
				meta := locales.TrackCountDot(len(al.Songs))
				if al.Released != "" {
					meta = " · " + al.Released + meta
				}
				if n := al.Length(); n > 0 {
					meta += " · " + albumLength(n)
				}
				ui.Text(c, meta)
			})
			if al.Label != "" {
				ui.Text(c, al.Label).FontSize(12.5).TextColor(p.muted).SingleLine().Margin(2, 0, 0, 0)
			}
			ui.Row(c).Gap(10).Wrap().Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, locales.Play(), p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(al.URI(), al.Songs, firstPlayable(al.Songs))
				}
				icon, label := iconPlus, locales.SaveAlbum()
				if al.Saved {
					icon, label = iconCheck, locales.AlbumSaved()
				}
				if pillButton(c.Key("save"), icon, label, p.hover.Over(p.bg), p.text).Clicked() {
					a.toggleAlbumSaved(al.ID)
				}
			})
		})
	})
}

// albumGrid shows albums as cards, each opening its page.
func (a *app) albumGrid(c *ui.Context, albums []music.AlbumProfile) {
	ui.Grid(c).Columns(a.columns(c, 156)).GapX(20).GapY(28).Children(func() {
		for i := range albums {
			a.albumCard(c.Key(albums[i].ID), &albums[i])
		}
	})
}

// albumCard is an album's cover, lifting under the pointer, over its name
// and the year it came out.
func (a *app) albumCard(c *ui.Context, al *music.AlbumProfile) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(al.Name)
	hovered := card.Hovered()
	card.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.AnimatePose(lift, ui.Snappy)
		art.Children(func() { a.coverFill(c, al.Images, 400, 10) })
		ui.Text(c, al.Name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		var meta []string
		if a.route.page != pageArtist {
			meta = append(meta, al.ArtistNames())
		}
		if y := al.Year(); y != "" {
			meta = append(meta, y)
		}
		ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
	})
	if card.Clicked() {
		a.open(route{page: pageAlbum, id: al.ID})
	}
}

// savedAlbumsPage is how many of the listener's saved albums load at a time.
const savedAlbumsPage = 50

// loadSavedAlbums loads the next page of the albums the listener saved.
func (a *app) loadSavedAlbums() {
	if a.savedAlbumsPage.loading {
		return
	}
	a.savedAlbumsPage.loading, a.savedAlbumsPage.err = true, ""
	offset := len(a.savedAlbums)
	go func() {
		albums, more, err := a.lib.SavedAlbums(savedAlbumsPage, offset)
		a.post(func() {
			a.savedAlbumsPage.loading = false
			if err != nil {
				a.savedAlbumsPage.err = failed(err, locales.SavedAlbumsLoadFailed())
				return
			}
			a.savedAlbums = append(a.savedAlbums, albums...)
			a.savedAlbumsPage.loaded, a.savedAlbumsPage.more = true, more && len(albums) > 0
		})
	}()
}

// savedAlbumsView lists the albums the listener saved, the latest first.
func (a *app) savedAlbumsView(c *ui.Context) {
	a.scrollPage(c, func() {
		ui.Text(c, locales.SavedAlbums()).FontSize(30).FontWeight(800).LetterSpacing(-0.6).Margin(0, 0, 20, 0)
		a.albumGrid(c, a.savedAlbums)
		a.pageFoot(c, &a.savedAlbumsPage, a.loadSavedAlbums)
		if a.savedAlbumsPage.loaded && len(a.savedAlbums) == 0 {
			a.empty(c, locales.NoSavedAlbums())
		}
	})
}
