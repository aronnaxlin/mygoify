package app

import (
	_ "embed"
	"log"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygoify/locales"
)

// The app's icon in the menu bar on macOS, as the settings choose
// (appIcons), and always in the notification area on Windows and Linux.
// While it shows, closing the window leaves the music playing there, until
// the listener quits; the icon opens the window again.

// trayTemplate is the mark of the app's icon, black on
// transparent for macOS to tint: 15.6 points tall in the menu bar's 18, at
// 2×, as icon/tray draws it.
//
//go:embed tray/tray-template.png
var trayTemplate []byte

// trayColor is the app's icon, for the notification area.
//
//go:embed tray/tray.png
var trayColor []byte

// appIcons are where the app's icon shows on macOS, with their names: the
// Dock and the menu bar, the Dock alone, or the menu bar alone.
func appIcons() []struct{ id, label string } {
	return []struct{ id, label string }{
		{"dock-and-menubar", locales.AppIconDockAndMenubar()},
		{"dock", locales.AppIconDock()},
		{"menubar", locales.AppIconMenubar()},
	}
}

// showsTray tells whether the app's icon shows in the menu bar or the
// notification area.
func showsTray(s settings) bool {
	return runtime.GOOS != "darwin" || s.AppIcon != "dock"
}

// activationPolicy is macOS's for the settings: no Dock icon for the menu
// bar alone, unless a window is open, so the listener can switch to it
// with Command-Tab and the app's menu shows.
func activationPolicy(s settings, windowOpen bool) mygo.ActivationPolicy {
	if s.AppIcon == "menubar" && !windowOpen {
		return mygo.ActivationPolicyAccessory
	}
	return mygo.ActivationPolicyRegular
}

// windowOpen tells whether the main window or the settings window is
// open: shown or minimized, as the notch player doesn't count.
func (a *app) windowOpen() bool {
	if a.settingsWin != nil {
		return true
	}
	return a.win != nil && (a.win.IsVisible() || a.win.IsMinimized())
}

// syncDock shows or hides the Dock icon, as the settings and the open
// windows say.
func (a *app) syncDock() {
	if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(activationPolicy(a.settings, a.windowOpen()))
	}
}

// changeAppIcon shows the app's icon where the settings now say, as the
// listener chose another place for it.
func (a *app) changeAppIcon() {
	a.syncDock()
	a.applyTray()
}

// applyTray adds or removes the icon in the menu bar or notification area,
// as the settings say.
func (a *app) applyTray() {
	// The agent's icon, or Omarchy's bar's, opens the mini player.
	switch show := showsTray(a.settings) && !a.agent; {
	case show && a.tray == nil:
		icon, template := trayColor, false
		if runtime.GOOS == "darwin" {
			icon, template = trayTemplate, true
		}
		opts := mygo.TrayOptions{ID: "cloudmusic", Icon: icon, IconIsTemplate: template, ToolTip: mygo.App.Name()}
		if !trayMini() {
			opts.Menu = a.trayMenu()
		}
		t, err := mygo.NewTray(opts)
		if err != nil {
			log.Println("tray:", err)
			return
		}
		a.tray = t
		if trayMini() {
			// A click opens the mini player under the icon, as the agent's
			// icon does on Linux (DESIGN.md §4.3); a right click the menu.
			t.OnClick(a.toggleMini)
			t.OnRightClick(func() {
				t.SetMenu(a.trayMenu())
				t.PopUpMenu()
				t.SetMenu(nil)
			})
			return
		}
		// Windows' left click, where the right one opens the menu.
		t.OnClick(a.showMain)
	case !show && a.tray != nil:
		a.tray.Destroy()
		a.tray = nil
	}
}

// trayMenu is the icon's menu: the window, the player's controls,
// settings, updates, and quitting.
func (a *app) trayMenu() *mygo.Menu {
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			a.redraw()
		}
	}
	// In front, as from the menu bar alone, where the app may not be.
	front := func(fn func()) func() {
		return func() {
			mygo.App.Focus()
			fn()
		}
	}
	name := mygo.App.Name()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayOpenApp(name), Click: run(a.showMain)},
		mygo.Separator(),
		{Label: locales.PlayPause(), Click: run(func() { a.pl.TogglePlay() })},
		{Label: locales.NextTrack(), Click: run(func() { a.pl.Next() })},
		{Label: locales.PrevTrack(), Click: run(func() { a.pl.Prev() })},
		{Label: locales.MenuLikeOrUnlike(), Click: run(a.likeCurrent)},
		mygo.Separator(),
		{Label: locales.SettingsEllipsis(), Click: run(front(a.openSettings))},
		{Label: locales.MenuCheckForUpdates(), Click: run(front(checkForUpdates))},
		mygo.Separator(),
		{Label: locales.TrayQuitApp(name), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

// trayMini says whether a click on the menu bar icon opens the mini player
// in a flyout under it: on macOS, whose menu bar places one natively. On
// Linux the agent's icon opens it, in a process of its own.
func trayMini() bool { return runtime.GOOS == "darwin" }

// miniClosedAt is when the flyout of the mini player last closed: a click
// on the icon closes it first, as AppKit closes a popover on a click on
// what opened it, and the click then reaching the icon doesn't open it
// again.
var miniClosedAt time.Time

// toggleMini opens the mini player under the menu bar icon, or closes it.
func (a *app) toggleMini() {
	if a.miniFly != nil {
		a.closeMini()
		return
	}
	if time.Since(miniClosedAt) < 300*time.Millisecond {
		return
	}
	fly := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Width:     miniWidth,
		Height:    miniHeight,
		Focusable: true,
		Popover:   true,
		Content:   ui.View(a.miniView),
	})
	a.miniFly = fly
	fly.OnClose(func(*mygo.CloseEvent) {
		if a.miniFly == fly {
			a.miniFly = nil
		}
		miniClosedAt = time.Now()
	})
}
