package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygoify/locales"
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
		ui.Text(c, locales.OnboardingTitle()).FontSize(20).FontWeight(700)
		ui.Text(c, locales.OnboardingSubtitle()).FontSize(13).TextColor(p.muted).Margin(0, 0, 10, 0)
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
					b := ui.ButtonBase(c.Key("redo")).Label(locales.ModifyStep(title)).Height(24).PaddingX(8).Radius(6).Center()
					if b.Hovered() {
						b.Background(p.hover)
					}
					b.Children(func() { ui.Text(c, locales.Modify()).FontSize(12.5).TextColor(p.muted) })
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
	a.onboardStep(c, stepApp, locales.StepAppTitle(), "Client ID "+id, func() {
		a.note(c, locales.StepAppNote())
		row := ui.Row(c).Margin(4, 0, 2, 0)
		row.Children(func() {
			if a.action(c, "dashboard", locales.OpenDeveloperDashboard(), false).Clicked() {
				openURL(developerDashboardURL)
			}
		})
		a.fieldLabel(c, "Client ID")
		in := ui.TextInput(c.Key("client-id"), &ob.clientID).Placeholder(locales.ClientIDPlaceholder()).Label(locales.FillClientID()).FontSize(13)
		a.fieldLabel(c, locales.RedirectURI())
		redirect := ui.TextInput(c.Key("redirect"), &ob.redirect).Label(locales.FillRedirectURI()).FontSize(13)
		a.problem(c, ob.appErr)
		ui.Row(c).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
			save := off(a.action(c, "save-app", locales.Save(), true), strings.TrimSpace(ob.clientID) == "")
			if save.Clicked() || in.Submitted() || redirect.Submitted() {
				a.saveApp()
			}
		})
	})
}

func (a *app) stepLogin(c *ui.Context) {
	p := a.pal
	ob := &a.ob
	summary := locales.LoggedIn()
	if a.live && a.profile != nil {
		summary = locales.LoggedInAs(a.profile.Name)
	}
	a.onboardStep(c, stepLogin, locales.StepLoginTitle(), summary, func() {
		a.note(c, locales.StepLoginNote())
		if ob.loginWaiting {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
				ui.Spinner(c).FontSize(16)
				ui.Text(c, locales.CompleteLoginInBrowser()).FontSize(13).TextColor(p.muted).Grow(1)
				if a.action(c, "cancel-login", locales.Cancel(), false).Clicked() {
					a.cancelLogin()
				}
			})
			return
		}
		a.problem(c, ob.loginErr)
		ui.Row(c).Margin(6, 0, 0, 0).Children(func() {
			login := off(pillButton(c.Key("log-in"), iconUser, locales.LoginInBrowser(), p.accent, p.onAccent).Height(32), !ob.have.app)
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
	a.onboardStep(c, stepSoloist, locales.StepSoloistTitle(), a.settings.Spotify.withEnv().SoloistBin, func() {
		a.note(c, locales.StepSoloistNote())
		a.problem(c, ob.binErr)
		file := soloistArchive()
		switch {
		case dl.busy:
			a.downloadProgress(c)
			return
		case dl.confirm:
			a.note(c, locales.DownloadSoloistConfirm(file))
			ui.Row(c).Gap(10).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
				if a.action(c, "download-no", locales.Cancel(), false).Clicked() {
					dl.confirm = false
				}
				if a.action(c, "download-yes", locales.Download(), true).Clicked() {
					a.startDownload()
				}
			})
			return
		}
		ui.Row(c).Gap(10).Wrap().Margin(6, 0, 0, 0).Children(func() {
			if file != "" && a.action(c, "download", locales.DownloadSoloist(), true).Clicked() {
				a.askDownload()
			}
			if a.action(c, "choose", locales.ChooseDownloadedFile(), false).Clicked() {
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
				link("browser", locales.DownloadInBrowser(), soloistCDN+file)
			}
			link("official", locales.OfficialDownloadPage(), soloistDownloadsURL)
		})
	})
}

// downloadProgress shows how much of Soloist has come, with a way to give
// up, in the accent's fill.
func (a *app) downloadProgress(c *ui.Context) {
	p := a.pal
	dl := &a.ob.dl
	frac, label := float32(0), locales.PreparingDownload()
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
		if a.action(c, "cancel-download", locales.Cancel(), false).Clicked() {
			a.cancelDownload()
		}
	})
}

func (a *app) stepKey(c *ui.Context) {
	p := a.pal
	ob := &a.ob
	summary := locales.KeySaved()
	switch ob.keyState {
	case keyValid:
		summary = locales.KeyValidSaved()
	case keyUnverified:
		summary = locales.KeyUnverifiedSaved()
	}
	a.onboardStep(c, stepKey, locales.StepKeyTitle(), summary, func() {
		a.note(c, locales.StepKeyNote())
		ui.Row(c).Margin(4, 0, 0, 0).Children(func() {
			if a.action(c, "key-page", locales.OpenKeyPage(), false).Clicked() {
				openURL(soloistKeyURL)
			}
		})
		in := ui.TextInput(c.Key("api-key"), &ob.key).Password().Placeholder("API key").Label(locales.FillAPIKey()).FontSize(13).Margin(4, 0, 0, 0)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
			switch ob.keyState {
			case keyChecking:
				ui.Spinner(c).FontSize(16)
				ui.Text(c, locales.VerifyingMaxWait()).FontSize(13).TextColor(p.muted).Grow(1)
				if a.action(c, "cancel-key", locales.Cancel(), false).Clicked() {
					a.cancelKey()
				}
			case keyInvalid:
				ui.Text(c, locales.KeyInvalid()).FontSize(13).TextColor(p.danger)
			case keyFailed:
				ui.Text(c, locales.KeyFailed()).FontSize(13).TextColor(p.danger)
			case keyValid:
				ui.Text(c, locales.KeyValid()).FontSize(13).TextColor(p.muted)
			case keyUnverified:
				ui.Text(c, locales.KeyUnverifiedSaved()).FontSize(13).TextColor(p.muted)
			}
			if ob.keyState != keyChecking {
				ui.Box(c).Grow(1)
			}
			if ob.keyState == keyChecking {
				return
			}
			wait := strings.TrimSpace(ob.key) == "" || !ob.have.bin
			verify := off(a.action(c, "verify", locales.Verify(), true), wait)
			if verify.Clicked() || in.Submitted() {
				a.submitKey()
			}
		})
	})
}

func (a *app) stepPair(c *ui.Context) {
	p := a.pal
	ob := &a.ob
	a.onboardStep(c, stepPair, locales.StepPairTitle(), locales.SoloistLoggedIn(), func() {
		if !ob.have.bin || !ob.have.key {
			a.note(c, locales.CompletePriorSteps())
			return
		}
		a.note(c, locales.PairInstructions(a.settings.Spotify.withEnv().DeviceName))
		ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
			ui.Spinner(c).FontSize(16)
			ui.Text(c, locales.WaitingForSoloistLogin()).FontSize(13).TextColor(p.muted).Grow(1)
			if a.action(c, "redo-key", locales.RedoKey(), false).Clicked() {
				a.redo(stepKey)
			}
		})
		ui.Text(c, locales.PairKeyMayBeWrong()).FontSize(12).TextColor(p.faint)
	})
}

// onboardingFoot closes the dialog, or starts the app again to play
// through what was set up.
func (a *app) onboardingFoot(c *ui.Context) {
	p := a.pal
	note := ""
	switch {
	case a.restartNeeded():
		note = locales.PlayWithYourSpotifyAfterRestart()
	case !a.live:
		note = locales.SampleMusicBeforeConnected()
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(12, 0, 0, 0).Children(func() {
		ui.Text(c, note).FontSize(12).TextColor(p.faint).Grow(1).Shrink(1)
		switch {
		case a.restartNeeded():
			if a.action(c, "later", locales.Later(), false).Clicked() {
				a.closeOnboarding()
			}
			if a.action(c, "restart", locales.Restart(), true).Clicked() {
				// The app comes back at once: Soloist plays on.
				a.relaunching = true
				relaunch()
			}
		case a.complete():
			if a.action(c, "done", locales.Done(), true).Clicked() {
				a.closeOnboarding()
			}
		default:
			if a.action(c, "dismiss", locales.Dismiss(), false).Clicked() {
				a.dismiss()
			}
		}
	})
}
