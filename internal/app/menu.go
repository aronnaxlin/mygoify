package app

import (
	"runtime"

	"github.com/egoist/mygo"

	"mygoify/locales"
)

// setMenu gives the app its menu bar, in the app's language and named as
// the app is, with the settings in the app menu and the player's controls
// in a menu of their own.
func (a *app) setMenu() {
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			a.redraw()
		}
	}
	name := mygo.App.Name()
	appMenu := []*mygo.MenuItem{
		{Role: mygo.RoleAbout, Label: locales.MenuAbout(name)},
		{Label: locales.MenuCheckForUpdates(), Click: run(checkForUpdates)},
		mygo.Separator(),
		{Label: locales.MenuSettings(), Accelerator: "CmdOrCtrl+,", Click: run(a.openSettings)},
		mygo.Separator(),
	}
	// Services and hiding are macOS's alone. Elsewhere MyGo leaves their
	// items out but keeps the separators around them, which then stack.
	if runtime.GOOS == "darwin" {
		appMenu = append(appMenu,
			&mygo.MenuItem{Role: mygo.RoleServices, Label: locales.MenuServices()},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleHide, Label: locales.MenuHide(name)},
			&mygo.MenuItem{Role: mygo.RoleHideOthers, Label: locales.MenuHideOthers()},
			&mygo.MenuItem{Role: mygo.RoleUnhide, Label: locales.MenuShowAll()},
			mygo.Separator(),
		)
	}
	appMenu = append(appMenu, &mygo.MenuItem{Role: mygo.RoleQuit, Label: locales.MenuQuit(name)})
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: name, Submenu: appMenu},
		{Label: locales.MenuFile(), Submenu: []*mygo.MenuItem{
			{Label: locales.MenuNewPlaylist(), Accelerator: "CmdOrCtrl+N", Click: run(func() {
				a.showMain()
				a.nowPlaying = false
				a.newPlaylist(nil)
			})},
		}},
		{Label: locales.MenuEdit(), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo, Label: locales.MenuUndo()},
			{Role: mygo.RoleRedo, Label: locales.MenuRedo()},
			mygo.Separator(),
			{Role: mygo.RoleCut, Label: locales.MenuCut()},
			{Role: mygo.RoleCopy, Label: locales.MenuCopy()},
			{Role: mygo.RolePaste, Label: locales.MenuPaste()},
			{Role: mygo.RoleSelectAll, Label: locales.MenuSelectAll()},
		}},
		{Label: locales.MenuControls(), Submenu: []*mygo.MenuItem{
			{Label: locales.MenuPlayPause(), Click: run(func() { a.pl.TogglePlay() })},
			{Label: locales.MenuNextTrack(), Accelerator: "CmdOrCtrl+Right", Click: run(func() { a.pl.Next() })},
			{Label: locales.MenuPrevTrack(), Accelerator: "CmdOrCtrl+Left", Click: run(func() { a.pl.Prev() })},
			mygo.Separator(),
			{Label: locales.MenuLikeOrUnlike(), Accelerator: "CmdOrCtrl+D", Click: run(a.likeCurrent)},
			mygo.Separator(),
			{Label: locales.MenuShuffle(), Click: run(func() { a.pl.ToggleShuffle() })},
			{Label: locales.MenuCycleRepeat(), Click: run(func() { a.pl.CycleRepeat() })},
			mygo.Separator(),
			{Label: locales.MenuNowPlaying(), Accelerator: "CmdOrCtrl+L", Click: run(func() {
				a.showMain()
				a.nowPlaying = !a.nowPlaying && a.pl.Current() != nil
			})},
		}},
		{Label: locales.MenuWindow(), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize, Label: locales.MenuMinimize()},
			{Role: mygo.RoleZoom, Label: locales.MenuZoom()},
			{Role: mygo.RoleClose, Label: locales.MenuClose()},
			mygo.Separator(),
			{Label: name, Accelerator: "CmdOrCtrl+0", Click: run(a.showMain)},
			{Role: mygo.RoleFront, Label: locales.MenuBringAllToFront()},
		}},
	}))
}
