package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// searchField is the search field atop the sidebar, which searches as
// Enter is pressed and, empty, lists the searches before. It suggests
// nothing as the query is typed, which would spend the Web API's quota.
func (a *app) searchField(c *ui.Context) {
	p := a.pal
	field := ui.Row(c).Height(32).PaddingX(10).Gap(7).Radius(9).AlignItems(ui.Center).Margin(0, 0, 14, 0)
	if c.Focused(a.searchBox) {
		field.Background(p.bg).Shadow(0, 0, 0, 3, p.text.Alpha(0.06)).Border(1, p.text.Alpha(0.3))
	} else {
		field.Background(p.hover).Border(1, p.hairline)
	}
	field.Children(func() {
		ui.Icon(c, iconSearch).Size(14, 14).TextColor(p.muted)
		cb := ui.ComboboxBase(c.Key("search"), &a.query)
		in := cb.Input.Bind(&a.searchBox).Grow(1).MinWidth(0).
			Placeholder("搜索歌曲、歌手、专辑").FontSize(13).Label("搜索")
		q := strings.TrimSpace(a.query)
		// Empty, it lists the searches before, if any.
		if q != "" || len(a.settings.SearchHistory) == 0 {
			cb.SetOpen(false)
		}
		cb.Popup(func(panel ui.Element) {
			panel.AttachTo(field, ui.AnchorBottomStart, ui.AnchorTopStart).
				Width(360).Padding(6).Margin(8, 0, 0, 0).Radius(12).
				Background(p.bg).Border(1, p.hairline).
				Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2))
			option := func(key, label string) ui.Element {
				row := cb.Item(key).MinHeight(36).Padding(6, 10).Gap(10).Radius(7).AlignItems(ui.Center).Label(label)
				if row.Highlighted() {
					row.Background(p.hover)
				}
				return row
			}
			panel.Label("搜索历史")
			ui.Row(c).Padding(2, 4, 2, 10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "搜索历史").FontSize(12).TextColor(p.muted).Grow(1)
				iconButton(c.Key("clear"), iconTrash, 14, p.muted, "清空搜索历史").KeepFocus().OnClick(a.clearSearchHistory)
			})
			for _, h := range a.settings.SearchHistory {
				option("history:"+h, h).Children(func() {
					ui.Icon(c, iconSearch).Size(13, 13).TextColor(p.muted)
					ui.Text(c, h).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
				})
			}
		})
		if key, ok := cb.Chosen(); ok {
			cb.SetOpen(false)
			a.choose(key)
		} else if in.Submitted() && q != "" {
			cb.SetOpen(false)
			a.search(q)
		}
	})
}

// choose searches again for the search chosen in the search field's
// history.
func (a *app) choose(key string) {
	if h, ok := strings.CutPrefix(key, "history:"); ok {
		a.query = h
		a.search(h)
	}
}
