package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
	"mygoify/locales"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *music.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, locales.More())
	b.Menu(a.songItems(tr, ""))
	return b
}

// songItems builds the menu of what can be done with tr, behind its dots
// or a right click on its row: queueing it, playing its radio, copying its
// title, its title
// with its artists, or the link to its page, opening its artists' or
// album's, and, signed in, adding it to a playlist or taking it out of
// pid, the playlist it is listed in, if set.
func (a *app) songItems(tr *music.Track, pid string) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		if m.Item(locales.AddToQueue()).Chosen() {
			a.addToQueue(tr)
		}
		if m.Item(locales.StartRadio()).Chosen() {
			a.startRadio(tr.URI(), tr)
		}
		m.Separator()
		if a.profile != nil {
			label := locales.Like()
			if a.isLiked(tr) {
				label = locales.Unlike()
			}
			if m.Item(label).Chosen() {
				a.toggleLike(tr)
			}
			m.Separator()
		}
		if m.Item(locales.CopySongTitle()).Chosen() {
			copyText(tr.Name)
		}
		if m.Item(locales.CopyTitleAndArtist()).Chosen() {
			copyText(tr.Name + " - " + tr.ArtistNames())
		}
		m.Separator()
		if m.Item(locales.CopyLink()).Chosen() {
			copyText(tr.URL())
		}
		// Its artists' and album's pages, those Spotify has.
		linked := tr.Album.ID != ""
		for _, ar := range tr.Artists {
			linked = linked || ar.ID != ""
		}
		if linked {
			m.Separator()
		}
		for _, ar := range tr.Artists {
			if ar.ID != "" && m.Item(locales.ViewArtist(ar.Name)).Chosen() {
				a.openArtist(ar.ID)
			}
		}
		if tr.Album.ID != "" && m.Item(locales.ViewAlbum(tr.Album.Name)).Chosen() {
			a.openPage(route{page: pageAlbum, id: tr.Album.ID})
		}
		a.playlistItems(m, tr, pid)
	}
}

// addToQueue queues tr, and says with a toast whether it was.
func (a *app) addToQueue(tr *music.Track) {
	a.pl.AddToQueue(*tr, func(err error) {
		if err != nil {
			a.toast = locales.AddToQueueFailed()
		} else {
			a.toast = locales.AddedToQueue()
		}
	})
}
