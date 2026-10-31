package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *music.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, "更多")
	b.Menu(a.songItems(tr, ""))
	return b
}

// songItems builds the menu of what can be done with tr, behind its dots
// or a right click on its row: queueing it, copying its title, its title
// with its artists, or the link to its page, opening its artists' or
// album's, and, signed in, adding it to a playlist or taking it out of
// pid, the playlist it is listed in, if set.
func (a *app) songItems(tr *music.Track, pid string) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		if m.Item("加入队列").Chosen() {
			a.addToQueue(tr)
		}
		m.Separator()
		if a.profile != nil {
			label := "喜欢"
			if a.isLiked(tr) {
				label = "取消喜欢"
			}
			if m.Item(label).Chosen() {
				a.toggleLike(tr)
			}
			m.Separator()
		}
		if m.Item("复制歌名").Chosen() {
			copyText(tr.Name)
		}
		if m.Item("复制歌名和歌手").Chosen() {
			copyText(tr.Name + " - " + tr.ArtistNames())
		}
		m.Separator()
		if m.Item("复制链接").Chosen() {
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
			if ar.ID != "" && m.Item("查看歌手："+ar.Name).Chosen() {
				a.openArtist(ar.ID)
			}
		}
		if tr.Album.ID != "" && m.Item("查看专辑："+tr.Album.Name).Chosen() {
			a.openPage(route{page: pageAlbum, id: tr.Album.ID})
		}
		a.playlistItems(m, tr, pid)
	}
}

// addToQueue queues tr, and says with a toast whether it was.
func (a *app) addToQueue(tr *music.Track) {
	a.pl.AddToQueue(*tr, func(err error) {
		if err != nil {
			a.toast = "加入队列失败，请稍后再试"
		} else {
			a.toast = "已加入队列"
		}
	})
}
