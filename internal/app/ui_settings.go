package app

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygoify/locales"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           locales.Settings(),
		Width:           560,
		Height:          720,
		MinWidth:        480,
		MinHeight:       420,
		StateKey:        "settings",
		DisableMaximize: true,
		DisableMinimize: true,
		// The app's menu has nothing for this window: on Linux and
		// Windows it stays out of sight until Alt shows it.
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f4f4f6, #1c1c1f)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
		a.syncDock()
	})
	a.syncDock()
}

// changed applies the settings everywhere and keeps them.
func (a *app) changed() {
	a.settings.save()
	a.settings.applyAppearance()
	if a.notch != nil {
		a.notch.setEnabled(a.settings.Notch)
	}
	a.redraw()
}

func (a *app) changeLanguage() {
	tag := a.settings.Language
	if mygo.App.IsReady() {
		mygo.App.SetLocale(tag)
	}
	if tag != "" {
		_ = os.Setenv("MYGO_LOCALE", tag)
	} else {
		_ = os.Unsetenv("MYGO_LOCALE")
	}
	saveLocaleFile(tag)
	a.setMenu()
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
	if a.settingsWin != nil {
		a.settingsWin.SetTitle(locales.Settings())
	}
	a.redraw()
}

func saveLocaleFile(tag string) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return
	}
	path := filepath.Join(dir, "locale")
	if tag == "" {
		_ = os.Remove(path)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(tag+"\n"), 0o644)
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	t.Accent, t.AccentText = p.accent, p.onAccent
	c.SetTheme(&t)
	c.Root().Background(p.sidebar)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.settingsWin.Close() })
	s := &a.settings

	ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8).Children(func() {
		// group is a card of settings, and under it a note of what isn't
		// plain from their names, if anything is, as macOS's settings do.
		group := func(title, note string, rows func()) {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(rows)
			if note != "" {
				ui.Text(c, note).FontSize(12).TextColor(p.muted).LineHeight(1.45).Margin(6, 16, 0, 16)
			}
		}
		row := func(label string, control func()) {
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
				ui.Text(c, label).FontSize(13.5).FontWeight(500).Grow(1).Shrink(1).MinWidth(0)
				control()
			})
		}

		// Only on macOS: elsewhere the icon is always in the notification
		// area.
		if runtime.GOOS == "darwin" {
			group(locales.General(), "", func() {
				row(locales.AppIcon(), func() {
					icons := appIcons()
					labels := make([]string, len(icons))
					chosen := icons[0].label
					for i, ic := range icons {
						labels[i] = ic.label
						if ic.id == s.AppIcon {
							chosen = ic.label
						}
					}
					if ui.Select(c.Key("app-icon"), &chosen, labels).Label(locales.AppIcon()).Width(150).Changed() {
						for _, ic := range icons {
							if ic.label == chosen {
								s.AppIcon = ic.id
							}
						}
						a.changed()
						a.changeAppIcon()
					}
				})
			})
		}

		// What happens to the music as the app goes (DESIGN.md §4.2):
		// Soloist plays apart from the window. Soloist has no quality to
		// set: Spotify keeps one for each Connect device, chosen in its own
		// apps but not on the web, and Soloist played at 160 kbps until it
		// was (2026-11-02).
		device := s.Spotify.withEnv().DeviceName
		group(locales.Playback(), locales.PlaybackQualityNote(device), func() {
			row(locales.Quality(), func() {
				ui.Text(c, locales.SelectInOfficialApp()).FontSize(12.5).TextColor(p.muted)
			})
			modes := quitModes()
			mode := modes[0]
			for _, m := range modes {
				if m.id == s.Quit {
					mode = m
				}
			}
			row(locales.WhenQuittingApp(), func() {
				labels := make([]string, len(modes))
				for i, m := range modes {
					labels[i] = m.label
				}
				chosen := mode.label
				if ui.Select(c.Key("quit"), &chosen, labels).Label(locales.WhenQuittingApp()).Width(170).Changed() {
					for _, m := range modes {
						if m.label == chosen {
							s.Quit = m.id
						}
					}
					a.changed()
				}
			})
			row(locales.ClickPlaysSong(), func() {
				if ui.Switch(c.Key("click-plays"), &s.ClickPlays).Label(locales.ClickPlaysSong()).Changed() {
					a.changed()
				}
			})
			if a.canHide() {
				row(locales.CloseWindowQuits(), func() {
					if ui.Switch(c.Key("close-quits"), &s.CloseQuits).Label(locales.CloseWindowQuits()).Changed() {
						a.changed()
					}
				})
			}
			row(locales.StartSoloistAtLogin(), func() {
				if ui.Switch(c.Key("soloist-at-login"), &s.Spotify.SoloistAtLogin).Label(locales.StartSoloistAtLogin()).Changed() {
					a.changed()
					a.changeAtLogin()
				}
			})
		})

		group(locales.Lyrics(), locales.LyricsSourcesNote(), func() {
			row(locales.LyricsServer(), func() {
				if ui.TextInput(c.Key("lyrics-server"), &s.LyricsServer).Placeholder("https://lrclib.net").Label(locales.LyricsServer()).FontSize(13).Width(200).Changed() {
					a.changed()
				}
			})
			row(locales.NeteaseCloudMusic(), func() {
				if ui.Switch(c.Key("lyrics-netease"), &s.LyricsNetEase).Label(locales.NeteaseCloudMusic()).Changed() {
					a.lyricsSourceChanged()
				}
			})
			row(locales.QqMusic(), func() {
				if ui.Switch(c.Key("lyrics-qq"), &s.LyricsQQ).Label(locales.QqMusic()).Changed() {
					a.lyricsSourceChanged()
				}
			})
		})

		// Only on macOS, the one with a notch player.
		if a.notch != nil {
			group(locales.NotchPlayer(), "", func() {
				row(locales.NotchPlayer(), func() {
					if ui.Switch(c.Key("notch"), &s.Notch).Label(locales.NotchPlayer()).Changed() {
						a.changed()
					}
				})
				row(locales.ShowActivityWhilePlaying(), func() {
					if ui.Switch(c.Key("activity"), &s.NotchActivity).Label(locales.ShowActivityWhilePlaying()).Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
				row(locales.PersistentOnNotchlessScreens(), func() {
					if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label(locales.PersistentOnNotchlessScreens()).Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
			})
		}

		group(locales.Appearance(), "", func() {
			row(locales.Appearance(), func() {
				modes := []string{"system", "light", "dark"}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, locales.FollowSystem(), locales.Light(), locales.Dark()) {
					s.Appearance = modes[i]
					a.changed()
				}
			})
			row(locales.Language(), func() {
				options := []struct{ id, label string }{
					{"", locales.FollowSystem()},
					{"zh-Hans", "简体中文"},
					{"zh-Hant", "繁體中文"},
					{"en", "English"},
					{"ja", "日本語"},
				}
				labels := make([]string, len(options))
				chosen := options[0].label
				for k, opt := range options {
					labels[k] = opt.label
					if opt.id == s.Language {
						chosen = opt.label
					}
				}
				if ui.Select(c.Key("language"), &chosen, labels).Label(locales.SelectLanguage()).Width(150).Changed() {
					for _, opt := range options {
						if opt.label == chosen {
							s.Language = opt.id
						}
					}
					a.changed()
					a.changeLanguage()
				}
			})
		})

		group(locales.ExperimentalFeatures(), locales.ExperimentalNote(), func() {
			row(locales.ProgressBarSmoke(), func() {
				if ui.Switch(c.Key("smoke"), &s.Smoke).Label(locales.ProgressBarSmoke()).Changed() {
					a.changed()
				}
			})
			row(locales.FogBackdrop(), func() {
				if ui.Switch(c.Key("fog"), &s.Fog).Label(locales.FogBackdrop()).Changed() {
					a.changed()
				}
			})
		})

		// What Soloist is, and what its version has left: renaming it takes
		// the next start of the app, for the service file carries the name.
		note := a.soloistAbout() + locales.DeviceNameNote()
		group(locales.Account(), note, func() {
			row(locales.ConnectSpotify(), func() {
				if ui.Button(c.Key("onboarding"), locales.SettingsEllipsis()).Clicked() {
					a.showMain()
					a.openOnboarding()
					a.win.Invalidate()
				}
			})
			row(locales.DeviceName(), func() {
				if ui.TextInput(c.Key("device-name"), &s.Spotify.DeviceName).Placeholder("MyGoify").Label(locales.FillDeviceName()).FontSize(13).Width(200).Changed() {
					a.keepSettings()
				}
			})
			if a.profile == nil {
				row(locales.NotSignedIn(), func() {
					if ui.PrimaryButton(c.Key("login"), locales.Login()).Clicked() {
						a.showMain()
						a.openOnboarding()
						a.win.Invalidate()
					}
				})
				return
			}
			prof := a.profile
			ui.Row(c).Padding(12, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				a.avatar(c, prof.Images, 36, p.press)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, prof.Name).FontSize(13.5).FontWeight(600)
					ui.Text(c, locales.SignedInSpotify()).FontSize(12).TextColor(p.muted)
				})
				if ui.Button(c.Key("logout"), locales.SignOut()).Clicked() {
					a.signOut()
				}
			})
		})

		if a.updates {
			var checked string
			if t := lastUpdateCheck(); !t.IsZero() {
				checked = locales.LastChecked(relativeTime(t, time.Now()))
			}
			group(locales.Updates(), checked, func() {
				row(locales.AutomaticallyCheckForUpdates(), func() {
					on := updateChecks()
					if ui.Switch(c.Key("update-checks"), &on).Label(locales.AutomaticallyCheckForUpdates()).Changed() {
						setUpdateChecks(on)
					}
				})
				row(locales.AutomaticallyDownloadAndInstall(), func() {
					on := updateDownloads()
					if ui.Switch(c.Key("update-downloads"), &on).Label(locales.AutomaticallyDownloadAndInstall()).Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
				row(locales.CurrentVersion(mygo.App.Version()), func() {
					if ui.Button(c.Key("check-updates"), locales.CheckForUpdates()).Clicked() {
						checkForUpdates()
					}
				})
			})
		}
	})
}
