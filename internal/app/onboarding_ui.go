package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// onboardingDialog walks the listener through connecting Spotify: each
// step shows what it has set up, and can be done again alone.
func (a *app) onboardingDialog(c *ui.Context) {
	p := a.pal
	ob := &a.ob
	// Closed by Escape or a click outside: what was under way is dropped.
	if ob.shown && !ob.open {
		a.stopOnboarding()
	}
	ob.shown = ob.open
	ui.DialogBase(c.Key("onboarding"), &ob.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(460).Padding(26, 26, 22, 26).Radius(22).Gap(4).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "连接 Spotify").FontSize(20).FontWeight(700)
		ui.Text(c, "分五步，每一步都可以单独重做").FontSize(13).TextColor(p.muted).Margin(0, 0, 10, 0)
		// In a window as short as the app allows, the steps scroll.
		_, h := c.Size()
		ui.Scroll(c).MaxHeight(max(h-230, 200)).Children(func() {
			ui.Column(c).Dividers(1, p.hairline).Children(func() {
				a.stepApp(c)
				a.stepLogin(c)
				a.stepSoloist(c)
				a.stepKey(c)
				a.stepPair(c)
			})
		})
		a.onboardingFoot(c)
	})
}

// onboardStep is one step: its number, or a check once it is done, its
// title and, as it is not the one open, what it has set up with a button
// to do it again; as it is, its controls.
func (a *app) onboardStep(c *ui.Context, i int, title, summary string, body func()) {
	p := a.pal
	ob := &a.ob
	open, done := ob.step == i, a.done(i)
	ui.Row(c.Key(title)).Gap(12).Padding(12, 0).AlignItems(ui.Start).Children(func() {
		mark := ui.Box(c).Size(24, 24).Radius(12).Center().Background(p.press).Margin(1, 0, 0, 0)
		if done {
			mark.Background(p.accent)
		}
		mark.Children(func() {
			if done {
				ui.Icon(c, iconCheck).Size(13, 13).TextColor(p.onAccent)
			} else {
				ui.Text(c, strconv.Itoa(i+1)).FontSize(12).FontWeight(600).TextColor(p.muted)
			}
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Row(c).AlignItems(ui.Center).MinHeight(26).Children(func() {
				t := ui.Text(c, title).FontSize(14).FontWeight(600).Grow(1)
				if !open && !done {
					t.TextColor(p.muted)
				}
				if !open && done {
					b := ui.ButtonBase(c.Key("redo")).Label("修改" + title).Height(24).PaddingX(8).Radius(6).Center()
					if b.Hovered() {
						b.Background(p.hover)
					}
					b.Children(func() { ui.Text(c, "修改").FontSize(12.5).TextColor(p.muted) })
					if b.Clicked() {
						a.redo(i)
					}
				}
			})
			if open {
				body()
			} else if done && summary != "" {
				ui.Text(c, summary).FontSize(12.5).TextColor(p.muted).SingleLine().MinWidth(0)
			}
		})
	})
}

// note is a few lines of explanation under a step's title.
func (a *app) note(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(12.5).TextColor(a.pal.muted).LineHeight(1.45)
}

// fieldLabel names a field.
func (a *app) fieldLabel(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(12).FontWeight(500).TextColor(a.pal.muted).Margin(6, 0, 0, 0)
}

// problem says what went wrong, under what went wrong.
func (a *app) problem(c *ui.Context, s string) {
	if s != "" {
		ui.Text(c, s).FontSize(12.5).TextColor(a.pal.danger).Margin(2, 0, 0, 0)
	}
}

// action is a button of a step: the one that goes on in the accent's
// fill, the others beside it plain.
func (a *app) action(c *ui.Context, key string, label string, main bool) ui.Element {
	p := a.pal
	if main {
		return pillButton(c.Key(key), nil, label, p.accent, p.onAccent).Height(32)
	}
	return pillButton(c.Key(key), nil, label, p.hover.Over(p.bg), p.text).Height(32)
}

// off dims a button that can't be pressed yet.
func off(b ui.Element, disabled bool) ui.Element {
	if disabled {
		b.Disabled(true).Opacity(0.5)
	}
	return b
}

func (a *app) stepApp(c *ui.Context) {
	ob := &a.ob
	id := a.settings.Spotify.withEnv().ClientID
	if len(id) > 8 {
		id = id[:8] + "…"
	}
	a.onboardStep(c, stepApp, "创建应用", "Client ID "+id, func() {
		a.note(c, "每个人用自己的应用，不和别人共用额度。在 Spotify for Developers 新建一个应用，勾选 Web API，Redirect URI 填下面的回调地址，再把 Client ID 贴到这里。")
		row := ui.Row(c).Margin(4, 0, 2, 0)
		row.Children(func() {
			if a.action(c, "dashboard", "打开开发者后台", false).Clicked() {
				openURL(developerDashboardURL)
			}
		})
		a.fieldLabel(c, "Client ID")
		in := ui.TextInput(c.Key("client-id"), &ob.clientID).Placeholder("32 位字符").Label("填写 Client ID").FontSize(13)
		a.fieldLabel(c, "回调地址")
		redirect := ui.TextInput(c.Key("redirect"), &ob.redirect).Label("填写回调地址").FontSize(13)
		a.problem(c, ob.appErr)
		ui.Row(c).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
			save := off(a.action(c, "save-app", "保存", true), strings.TrimSpace(ob.clientID) == "")
			if save.Clicked() || in.Submitted() || redirect.Submitted() {
				a.saveApp()
			}
		})
	})
}

func (a *app) stepLogin(c *ui.Context) {
	p := a.pal
	ob := &a.ob
	summary := "已登录"
	if a.live && a.profile != nil {
		summary = "已登录为 " + a.profile.Name
	}
	a.onboardStep(c, stepLogin, "登录 Spotify", summary, func() {
		a.note(c, "在浏览器里登录，并允许 MyGoify 访问你的曲库。需要 Spotify Premium。")
		if ob.loginWaiting {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
				ui.Spinner(c).FontSize(16)
				ui.Text(c, "请在浏览器中完成登录").FontSize(13).TextColor(p.muted).Grow(1)
				if a.action(c, "cancel-login", "取消", false).Clicked() {
					a.cancelLogin()
				}
			})
			return
		}
		a.problem(c, ob.loginErr)
		ui.Row(c).Margin(6, 0, 0, 0).Children(func() {
			login := off(pillButton(c.Key("log-in"), iconUser, "在浏览器中登录", p.accent, p.onAccent).Height(32), !ob.have.app)
			if login.Clicked() {
				a.startLogin()
			}
		})
	})
}

func (a *app) stepSoloist(c *ui.Context) {
	p := a.pal
	ob := &a.ob
	dl := &ob.dl
	a.onboardStep(c, stepSoloist, "Soloist", a.settings.Spotify.withEnv().SoloistBin, func() {
		a.note(c, "Soloist 是 Spotify 官方的播放器，MyGoify 通过它播放。可以直接下载，也可以自己下载后选择。")
		a.problem(c, ob.binErr)
		file := soloistArchive()
		switch {
		case dl.busy:
			a.downloadProgress(c)
			return
		case dl.confirm:
			a.note(c, "将从 Spotify 官方（soloist-builds.spotifycdn.com）下载 "+file+"，解压后保存在这台电脑上。")
			ui.Row(c).Gap(10).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
				if a.action(c, "download-no", "取消", false).Clicked() {
					dl.confirm = false
				}
				if a.action(c, "download-yes", "下载", true).Clicked() {
					a.startDownload()
				}
			})
			return
		}
		ui.Row(c).Gap(10).Wrap().Margin(6, 0, 0, 0).Children(func() {
			if file != "" && a.action(c, "download", "下载 Soloist", true).Clicked() {
				a.askDownload()
			}
			if a.action(c, "choose", "选择已下载的文件…", false).Clicked() {
				a.pickSoloist()
			}
		})
		// For a browser to download it, or to see Spotify's own page.
		ui.Row(c).Gap(14).Margin(2, 0, 0, 0).Children(func() {
			link := func(key, label, url string) {
				b := ui.ButtonBase(c.Key(key)).Label(label).Height(24).PaddingX(6).Radius(6).Center()
				if b.Hovered() {
					b.Background(p.hover)
				}
				b.Children(func() { ui.Text(c, label).FontSize(12.5).TextColor(p.muted) })
				if b.Clicked() {
					openURL(url)
				}
			}
			if file != "" {
				link("browser", "用浏览器下载", soloistCDN+file)
			}
			link("official", "官方下载页", soloistDownloadsURL)
		})
	})
}

// downloadProgress shows how much of Soloist has come, with a way to give
// up, in the accent's fill.
func (a *app) downloadProgress(c *ui.Context) {
	p := a.pal
	dl := &a.ob.dl
	frac, label := float32(0), "正在准备…"
	if dl.total > 0 {
		frac = min(float32(dl.done)/float32(dl.total), 1)
		label = fmt.Sprintf("%.1f / %.1f MB", float64(dl.done)/1e6, float64(dl.total)/1e6)
	} else if dl.done > 0 {
		label = fmt.Sprintf("%.1f MB", float64(dl.done)/1e6)
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
		ui.Column(c).Grow(1).Gap(6).Children(func() {
			ui.Box(c).Height(6).Radius(3).Draw(func(pt *ui.Painter, r ui.Rect) {
				pt.Fill(r, p.press, 3)
				if frac > 0 {
					pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * frac, H: r.H}, p.accent, 3)
				}
			})
			ui.Text(c, label).FontSize(12).TextColor(p.muted)
		})
		if a.action(c, "cancel-download", "取消", false).Clicked() {
			a.cancelDownload()
		}
	})
}

func (a *app) stepKey(c *ui.Context) {
	p := a.pal
	ob := &a.ob
	summary := "已保存"
	switch ob.keyState {
	case keyValid:
		summary = "key 有效，已保存"
	case keyUnverified:
		summary = "已保存，配对后才能确认"
	}
	a.onboardStep(c, stepKey, "API key", summary, func() {
		a.note(c, "在开发者后台的 Soloist 页面生成 API key，贴到这里。key 只保存在这台电脑上，不会显示出来。")
		in := ui.TextInput(c.Key("api-key"), &ob.key).Password().Placeholder("API key").Label("填写 API key").FontSize(13).Margin(4, 0, 0, 0)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
			switch ob.keyState {
			case keyChecking:
				ui.Spinner(c).FontSize(16)
				ui.Text(c, "验证中，最多要 30 秒…").FontSize(13).TextColor(p.muted).Grow(1)
				if a.action(c, "cancel-key", "取消", false).Clicked() {
					a.cancelKey()
				}
			case keyInvalid:
				ui.Text(c, "key 无效，Spotify 拒绝了它").FontSize(13).TextColor(p.danger)
			case keyFailed:
				ui.Text(c, "Soloist 没能启动或登录，可以再试一次").FontSize(13).TextColor(p.danger)
			case keyValid:
				ui.Text(c, "key 有效").FontSize(13).TextColor(p.muted)
			case keyUnverified:
				ui.Text(c, "已保存，配对后才能确认").FontSize(13).TextColor(p.muted)
			}
			if ob.keyState != keyChecking {
				ui.Box(c).Grow(1)
			}
			if ob.keyState == keyChecking {
				return
			}
			wait := strings.TrimSpace(ob.key) == "" || !ob.have.bin
			verify := off(a.action(c, "verify", "验证", true), wait)
			if verify.Clicked() || in.Submitted() {
				a.submitKey()
			}
		})
	})
}

func (a *app) stepPair(c *ui.Context) {
	p := a.pal
	ob := &a.ob
	a.onboardStep(c, stepPair, "配对", "Soloist 已登录", func() {
		if !ob.have.bin || !ob.have.key {
			a.note(c, "先完成前面的步骤。")
			return
		}
		a.note(c, "在同一局域网的手机上打开 Spotify，选中设备“"+a.settings.Spotify.withEnv().DeviceName+"”，随便播放一首歌。")
		ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
			ui.Spinner(c).FontSize(16)
			ui.Text(c, "等待 Soloist 登录…").FontSize(13).TextColor(p.muted).Grow(1)
			if a.action(c, "redo-key", "重填 key", false).Clicked() {
				a.redo(stepKey)
			}
		})
		ui.Text(c, "等不到的话，key 可能填错了。").FontSize(12).TextColor(p.faint)
	})
}

// onboardingFoot closes the dialog, or starts the app again to play
// through what was set up.
func (a *app) onboardingFoot(c *ui.Context) {
	p := a.pal
	note := ""
	switch {
	case a.restartNeeded():
		note = "重新启动后，用你的 Spotify 播放"
	case !a.live:
		note = "连上之前，看到的是示例音乐"
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(12, 0, 0, 0).Children(func() {
		ui.Text(c, note).FontSize(12).TextColor(p.faint).Grow(1).Shrink(1)
		switch {
		case a.restartNeeded():
			if a.action(c, "later", "稍后", false).Clicked() {
				a.closeOnboarding()
			}
			if a.action(c, "restart", "重新启动", true).Clicked() {
				relaunch()
			}
		case a.complete():
			if a.action(c, "done", "完成", true).Clicked() {
				a.closeOnboarding()
			}
		default:
			if a.action(c, "dismiss", "稍后再说", false).Clicked() {
				a.dismiss()
			}
		}
	})
}
