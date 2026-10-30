package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// These sign in to Spotify, in the browser, and out; tests replace them.
// Until internal/auth comes (M2) the fake library is signed in at once.
var (
	// restoreLogin signs in with the login kept from before, if any.
	restoreLogin = func() bool { return true }
	// logIn opens the browser for the listener to log in, and returns once
	// they have.
	logIn  = func() error { return nil }
	logOut = func() {}
)

// loginState is a login under way, in the browser.
type loginState struct {
	open     bool
	gen      int  // counts logins, to drop the answers of closed ones
	waiting  bool // the browser is open, for the listener to log in
	err      string
	finished bool
}

// boot signs in with the login kept, if any, then loads the home page for
// whoever is listening.
func (a *app) boot() {
	a.homeLoading = true
	go func() {
		var prof *music.User
		if restoreLogin() {
			prof, _ = a.lib.Me()
		}
		a.post(func() {
			a.signedIn(prof)
		})
	}()
}

// signedIn shows what is personal to prof, or to nobody.
func (a *app) signedIn(prof *music.User) {
	a.profile = prof
	a.mine, a.saved, a.liked = nil, nil, nil
	a.home = music.Home{}
	// What the listener may play, and follows, changed: load the pages
	// again.
	a.playlists = map[string]*music.Playlist{}
	a.plErr = map[string]string{}
	a.artists, a.albums = map[string]*artistData{}, map[string]*albumData{}
	a.searches = map[route]*searchData{}
	if prof == nil {
		a.homeLoading = false
		return
	}
	a.load(a.route)
	a.loadHome()
	a.loadLiked(prof.ID)
	a.loadMine(prof.ID)
}

func (a *app) openLogin() {
	a.login = loginState{open: true, gen: a.login.gen + 1}
}

// startLogin opens the browser for the listener to log in, and signs in
// once they have.
func (a *app) startLogin() {
	l := &a.login
	l.waiting, l.err = true, ""
	gen := l.gen
	go func() {
		err := logIn()
		var prof *music.User
		if err == nil {
			prof, err = a.lib.Me()
		}
		a.post(func() {
			l := &a.login
			if gen != l.gen || !l.open {
				return
			}
			l.waiting = false
			if err != nil {
				l.err = "登录失败，请稍后再试"
				return
			}
			l.finished = true
			a.signedIn(prof)
			a.closeLoginSoon(gen)
		})
	}()
}

// closeLoginSoon closes the dialog a moment after it showed the welcome.
func (a *app) closeLoginSoon(gen int) {
	go func() {
		time.Sleep(1200 * time.Millisecond)
		a.post(func() {
			if gen == a.login.gen {
				a.login.open = false
			}
		})
	}()
}

func (a *app) signOut() {
	a.login.gen++
	a.signedIn(nil)
	go logOut()
}

// loginDialog asks the listener to log in to Spotify, in the browser.
func (a *app) loginDialog(c *ui.Context) {
	p := a.pal
	l := &a.login
	ui.DialogBase(c.Key("login"), &l.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(340).Padding(28, 28, 26, 28).Radius(22).Gap(6).AlignItems(ui.Center).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		title, sub := "登录 Spotify", "在浏览器中登录你的 Spotify 账户"
		switch {
		case l.finished:
			title, sub = "登录成功", "欢迎回来，"+a.profileName()
		case l.err != "":
			sub = l.err
		case l.waiting:
			sub = "请在浏览器中完成登录"
		}
		ui.Text(c, title).FontSize(20).FontWeight(700)
		ui.Text(c, sub).FontSize(13).TextColor(p.muted).TextAlign(ui.Center).Margin(0, 0, 14, 0)
		switch {
		case l.finished:
			if a.profile != nil {
				a.avatar(c, a.profile.Images, 72, p.press)
			}
		case l.waiting:
			ui.Spinner(c).FontSize(22).Margin(6, 0)
		default:
			if pillButton(c.Key("log-in"), iconUser, "在浏览器中登录", p.accent, p.onAccent).Clicked() {
				a.startLogin()
			}
		}
		ui.Text(c, "需要 Spotify Premium").FontSize(11.5).TextColor(p.faint).Margin(16, 0, 0, 0)
	})
}

func (a *app) profileName() string {
	if a.profile == nil {
		return ""
	}
	return a.profile.Name
}

// accountRow sits at the foot of the sidebar: who is signed in, with a
// dropdown to open the settings or sign out, or a way to sign in.
func (a *app) accountRow(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c.Key("account")).Height(40).PaddingX(8).Gap(10).Radius(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || a.accountOpen && a.profile != nil {
		b.Background(p.hover)
	}
	if a.profile == nil {
		a.accountOpen = false
		b.Label("登录").Children(func() {
			ui.Box(c).Size(26, 26).Radius(13).Background(p.press).Center().Children(func() {
				ui.Icon(c, iconUser).Size(14, 14).TextColor(p.muted)
			})
			ui.Text(c, "登录 Spotify").FontSize(13).FontWeight(500).TextColor(p.muted)
		})
		if b.Clicked() {
			a.openLogin()
		}
		return
	}
	prof := a.profile
	b.Label(prof.Name).Expanded(a.accountOpen).Children(func() {
		a.avatar(c, prof.Images, 26, p.press)
		ui.Text(c, prof.Name).FontSize(13).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
	})
	if b.Clicked() {
		a.accountOpen = !a.accountOpen
	}
	a.accountMenu(c, b)
}

// accountMenu drops up from the account row, as wide as it, in the colors
// of the volume popover: the settings, then signing out.
func (a *app) accountMenu(c *ui.Context, row ui.Element) {
	pop := popoverColors(a.pal.dark)
	ui.PopoverBase(c.Key("account-menu"), row, &a.accountOpen, func(panel ui.Element) {
		panel.AttachTo(row, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).Padding(5).Gap(1).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label("账户").TransformOrigin(0.5, 1).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.96, Y: 4}, Exit: &ui.Motion{Scale: 0.98}})
		if w := row.Bounds().W; w > 0 {
			panel.Width(w)
		}
		shortcut := "Ctrl+,"
		if runtime.GOOS == "darwin" {
			shortcut = "⌘,"
		}
		if a.menuItem(c, iconSettings, "设置…", shortcut).AutoFocus().Clicked() {
			a.accountOpen = false
			a.openSettings()
		}
		ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
		if a.menuItem(c, iconSignOut, "退出登录", "").Clicked() {
			a.accountOpen = false
			a.signOut()
		}
	})
}

// menuItem is an item of a dropdown: an icon, its label, and the shortcut
// that does the same, if any.
func (a *app) menuItem(c *ui.Context, icon *ui.SVG, label, shortcut string) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c.Key(label)).Role(ui.RoleMenuItem).Label(label).Height(30).PaddingX(8).Gap(9).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.press)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(p.muted)
		ui.Text(c, label).FontSize(13).Grow(1)
		if shortcut != "" {
			ui.Text(c, shortcut).FontSize(12).TextColor(p.faint)
		}
	})
	return b
}

// myPlaylists lists the listener's playlists under a header that opens
// and closes them: those they made, if mine, with a button to make one
// and a menu to rename them.
func (a *app) myPlaylists(c *ui.Context, title string, lists []music.Playlist, mine bool, collapsed *bool) {
	if len(lists) == 0 {
		return
	}
	p := a.pal
	ui.Row(c.Key(title)).AlignItems(ui.Center).Margin(18, 4, 6, 6).Children(func() {
		b := ui.ButtonBase(c.Key("toggle")).Grow(1).Height(26).PaddingX(4).Gap(6).Radius(6).
			AlignItems(ui.Center).Justify(ui.Start).Role(ui.RoleDisclosure).Label(title)
		if b.Clicked() {
			*collapsed = !*collapsed
		}
		b.Expanded(!*collapsed)
		if b.Hovered() {
			b.Background(p.hover)
		}
		b.Children(func() {
			ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(p.faint).Grow(1)
			arrow := iconDown
			if *collapsed {
				arrow = iconForward
			}
			ui.Icon(c, arrow).Size(12, 12).TextColor(p.faint)
		})
		if mine && iconButton(c.Key("new"), iconPlus, 13, p.faint, "新建歌单").Size(26, 26).Radius(6).Clicked() {
			a.newPlaylist(nil)
		}
	})
	if *collapsed {
		return
	}
	for i := range lists {
		pl := &lists[i]
		selected := a.route.page == pagePlaylist && a.route.id == pl.ID && !a.nowPlaying
		b := ui.ButtonBase(c.Key(pl.ID)).Height(34).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
		switch {
		case selected:
			b.Background(p.press)
		case b.Hovered():
			b.Background(p.hover)
		}
		b.Children(func() {
			a.cover(c, pl.Images, 80, 22, 5)
			ui.Text(c, pl.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
		})
		if mine && !pl.Liked {
			b.ContextMenu(func(m *ui.Menu) {
				if m.Item("重命名…").Chosen() {
					a.startRename(pl)
				}
				m.Separator()
				if m.Item("删除歌单…").Chosen() {
					a.confirmDelete(pl)
				}
			})
		}
		if b.Clicked() {
			a.open(route{page: pagePlaylist, id: pl.ID})
		}
	}
}
