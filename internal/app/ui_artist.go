package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// artistLinks shows a track's artists as its credits read, each a link to
// their page in color, brightening to hover under the pointer, and then,
// with album, a link to its album. An artist Spotify has no page for,
// credited by name alone, is plain text.
func (a *app) artistLinks(c *ui.Context, tr *music.Track, color, hover ui.Color, album ...bool) ui.Element {
	return ui.RichText(c).TextColor(color).Children(func() {
		for i, ar := range tr.Artists {
			if i > 0 {
				ui.Text(c, ", ")
			}
			switch {
			case ar.ID != "":
				a.link(c.Key(i), ar.Name, color, hover, route{page: pageArtist, id: ar.ID})
			default:
				ui.Text(c, ar.Name)
			}
		}
		if len(album) > 0 && album[0] && tr.Album.Name != "" {
			ui.Text(c, " — ")
			switch {
			case tr.Album.ID != "":
				a.link(c.Key("album"), tr.Album.Name, color, hover, route{page: pageAlbum, id: tr.Album.ID})
			default:
				ui.Text(c, tr.Album.Name)
			}
		}
	})
}

// link is text in a RichText that opens a page.
func (a *app) link(c *ui.Context, text string, color, hover ui.Color, r route) {
	link := ui.Link(c, text, "").TextColor(color)
	if link.Hovered() {
		link.TextColor(hover)
	}
	// Once built: a song's row is in a list, which a new page resets.
	link.OnClick(func() { a.openPage(r) })
}

// openPage opens a page in the main window, from wherever a link to it
// shows: over Now Playing, which closes, or the notch.
func (a *app) openPage(r route) {
	a.open(r)
	a.showMain()
	a.redraw()
}

// openArtist opens an artist's page in the main window.
func (a *app) openArtist(id string) { a.openPage(route{page: pageArtist, id: id}) }

// artistData is what the app has of an artist: who they are, with their
// most popular songs, and their albums once asked for.
type artistData struct {
	profile    *music.ArtistProfile
	loading    bool
	err        string
	albums     []music.AlbumProfile
	albumsPage paged
	following  bool // a follow or unfollow is on its way
}

// What an artist's page lists: their most popular songs, or their albums.
const (
	artistHot = iota
	artistAlbums
)

// albumsPage is how many albums load at a time.
const albumsPage = 30

func (a *app) artist(id string) *artistData {
	d := a.artists[id]
	if d == nil {
		d = &artistData{}
		a.artists[id] = d
	}
	return d
}

// loadArtist loads an artist's page, and what it lists.
func (a *app) loadArtist(id string) {
	d := a.artist(id)
	if a.artistTab == artistAlbums && !d.albumsPage.loaded {
		a.loadAlbums(id)
	}
	if d.profile != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		ar, err := a.lib.Artist(id)
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err = failed(err, "歌手加载失败")
				return
			}
			d.profile = ar
		})
	}()
}

// loadAlbums loads the next page of an artist's albums.
func (a *app) loadAlbums(id string) {
	d := a.artist(id)
	if d.albumsPage.loading {
		return
	}
	d.albumsPage.loading, d.albumsPage.err = true, ""
	offset := len(d.albums)
	go func() {
		albums, more, err := a.lib.ArtistAlbums(id, albumsPage, offset)
		a.post(func() {
			d.albumsPage.loading = false
			if err != nil {
				d.albumsPage.err = failed(err, "专辑加载失败")
				return
			}
			d.albums = append(d.albums, albums...)
			d.albumsPage.loaded, d.albumsPage.more = true, more && len(albums) > 0
		})
	}()
}

// toggleFollow follows an artist, or unfollows them, at once, going back if
// Spotify refuses. Signed out, it asks the listener to sign in.
func (a *app) toggleFollow(id string) {
	d := a.artist(id)
	if d.profile == nil || d.following {
		return
	}
	if a.profile == nil {
		a.openLogin()
		return
	}
	ar := d.profile
	follow := !ar.Followed
	ar.Followed, d.following = follow, true
	go func() {
		err := a.lib.FollowArtist(id, follow)
		a.post(func() {
			d.following = false
			switch {
			case err != nil:
				ar.Followed = !follow
				a.toast = "操作失败，请稍后再试"
			case follow:
				a.toast = "已关注"
			default:
				a.toast = "已取消关注"
			}
		})
	}()
}

// artistPage shows who an artist is over their most popular songs, or
// their albums.
func (a *app) artistPage(c *ui.Context) {
	id := a.route.id
	d := a.artist(id)
	if d.profile == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadArtist(id) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.artistHeaderSkeleton(c, s) })
		return
	}
	header := func(c *ui.Context) { a.artistHeader(c, d) }
	switch a.artistTab {
	case artistAlbums:
		a.scrollPage(c, func() {
			header(c)
			a.albumGrid(c, d.albums)
			a.pageFoot(c, &d.albumsPage, func() { a.loadAlbums(id) })
			if d.albumsPage.loaded && len(d.albums) == 0 {
				a.empty(c, "暂无专辑")
			}
		})
		return
	}
	a.trackRows(c, d.profile.Songs, true, header)
}

// scrollPage scrolls a page of cards and rows, inset as one of tracks is:
// a tab of a page that lists tracks puts its header in the same place.
func (a *app) scrollPage(c *ui.Context, content func()) {
	top, bottom := a.insets(c)
	ui.Scroll(c.Key(a.route)).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		content()
	})
}

// empty says a list has nothing in it.
func (a *app) empty(c *ui.Context, msg string) {
	ui.Text(c, msg).FontSize(13).TextColor(a.pal.muted).Padding(40, 0).AlignSelf(ui.Center)
}

func (a *app) artistHeader(c *ui.Context, d *artistData) {
	p := a.pal
	ar := d.profile
	// What the buttons play: the most popular songs.
	songs := ar.Songs
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		photo := headerArt(c)
		ui.Box(c).Size(photo, photo).Radius(photo/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, ar.Images, 600, photo/2)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "歌手").FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, ar.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			if len(ar.Genres) > 0 {
				ui.Text(c, strings.Join(ar.Genres, " · ")).FontSize(13).TextColor(p.muted)
			}
			ui.Row(c).Gap(10).Wrap().Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue("", songs, firstPlayable(songs))
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					if !a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue("", songs, randomPlayable(songs))
				}
				icon, label := iconPlus, "关注"
				if ar.Followed {
					icon, label = iconCheck, "已关注"
				}
				if pillButton(c.Key("follow"), icon, label, p.hover.Over(p.bg), p.text).Clicked() {
					a.toggleFollow(ar.ID)
				}
			})
		})
	})
	ui.Row(c).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
		ui.Text(c, artistTabTitles[a.artistTab]).FontSize(19).FontWeight(700).LetterSpacing(-0.3)
		ui.Spacer(c)
		if segmented(c.Key("tab"), &a.artistTab, "热门", "专辑") {
			a.loadArtist(ar.ID)
		}
	})
}

var artistTabTitles = [...]string{"热门歌曲", "专辑"}

func (a *app) artistHeaderSkeleton(c *ui.Context, s shimmer) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		s.bone(c, headerArt(c)/2).Size(headerArt(c), headerArt(c))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			s.text(c, 12, 1.3).Width(24)
			s.text(c, 30, 1.2).WidthPercent(32)
			s.text(c, 13, 1.3).Width(160)
			ui.Column(c).Margin(2, 0, 0, 0).Children(func() {
				s.text(c, 12.5, 1.45).WidthPercent(78)
				s.text(c, 12.5, 1.45).WidthPercent(52)
			})
			ui.Row(c).Gap(10).Wrap().Margin(10, 0, 0, 0).Children(func() {
				s.bone(c, 18).Size(92, 36)
				s.bone(c, 18).Size(120, 36)
			})
		})
	})
	a.section(c, "歌曲")
}
