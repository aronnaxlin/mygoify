package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           "设置",
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
			group("通用", "", func() {
				row("应用图标", func() {
					labels := make([]string, len(appIcons))
					chosen := appIcons[0].label
					for i, ic := range appIcons {
						labels[i] = ic.label
						if ic.id == s.AppIcon {
							chosen = ic.label
						}
					}
					if ui.Select(c.Key("app-icon"), &chosen, labels).Label("应用图标").Width(150).Changed() {
						for _, ic := range appIcons {
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
		group("播放", "音质在手机或电脑上的 Spotify 官方 App 里选：点设备图标，选中 "+device+"，改成无损。只需一次，Soloist 更新后可能要再选；网页版不行。", func() {
			row("音质", func() {
				ui.Text(c, "在官方 App 里选").FontSize(12.5).TextColor(p.muted)
			})
			mode := quitModes[0]
			for _, m := range quitModes {
				if m.id == s.Quit {
					mode = m
				}
			}
			row("退出 App 时", func() {
				labels := make([]string, len(quitModes))
				for i, m := range quitModes {
					labels[i] = m.label
				}
				chosen := mode.label
				if ui.Select(c.Key("quit"), &chosen, labels).Label("退出 App 时").Width(170).Changed() {
					for _, m := range quitModes {
						if m.label == chosen {
							s.Quit = m.id
						}
					}
					a.changed()
				}
			})
			row("单击歌曲即播放", func() {
				if ui.Switch(c.Key("click-plays"), &s.ClickPlays).Label("单击歌曲即播放").Changed() {
					a.changed()
				}
			})
			if a.canHide() {
				row("关闭窗口时退出", func() {
					if ui.Switch(c.Key("close-quits"), &s.CloseQuits).Label("关闭窗口时退出").Changed() {
						a.changed()
					}
				})
			}
			row("登录时启动 Soloist", func() {
				if ui.Switch(c.Key("soloist-at-login"), &s.Spotify.SoloistAtLogin).Label("登录时启动 Soloist").Changed() {
					a.changed()
					a.changeAtLogin()
				}
			})
		})

		group("歌词", "网易云和 QQ 音乐是非公开接口，可能失效。", func() {
			row("歌词服务器", func() {
				if ui.TextInput(c.Key("lyrics-server"), &s.LyricsServer).Placeholder("https://lrclib.net").Label("歌词服务器").FontSize(13).Width(200).Changed() {
					a.changed()
				}
			})
			row("网易云音乐", func() {
				if ui.Switch(c.Key("lyrics-netease"), &s.LyricsNetEase).Label("网易云音乐").Changed() {
					a.lyricsSourceChanged()
				}
			})
			row("QQ 音乐", func() {
				if ui.Switch(c.Key("lyrics-qq"), &s.LyricsQQ).Label("QQ 音乐").Changed() {
					a.lyricsSourceChanged()
				}
			})
		})

		// Only on macOS, the one with a notch player.
		if a.notch != nil {
			group("刘海播放器", "", func() {
				row("刘海播放器", func() {
					if ui.Switch(c.Key("notch"), &s.Notch).Label("刘海播放器").Changed() {
						a.changed()
					}
				})
				row("播放时显示实时活动", func() {
					if ui.Switch(c.Key("activity"), &s.NotchActivity).Label("播放时显示实时活动").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
				row("无刘海的屏幕上常驻", func() {
					if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label("无刘海的屏幕上常驻").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
			})
		}

		group("外观", "", func() {
			row("外观", func() {
				modes := []string{"system", "light", "dark"}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, "跟随系统", "浅色", "深色") {
					s.Appearance = modes[i]
					a.changed()
				}
			})
		})

		group("实验性功能", "会多耗一些电。", func() {
			row("进度条烟雾", func() {
				if ui.Switch(c.Key("smoke"), &s.Smoke).Label("进度条烟雾").Changed() {
					a.changed()
				}
			})
			row("烟雾背景", func() {
				if ui.Switch(c.Key("fog"), &s.Fog).Label("烟雾背景").Changed() {
					a.changed()
				}
			})
		})

		// What Soloist is, and what its version has left: renaming it takes
		// the next start of the app, for the service file carries the name.
		note := a.soloistAbout() + "设备名改名后，重启 App 生效。"
		group("账号", note, func() {
			row("连接 Spotify", func() {
				if ui.Button(c.Key("onboarding"), "设置…").Clicked() {
					a.showMain()
					a.openOnboarding()
					a.win.Invalidate()
				}
			})
			row("设备名", func() {
				if ui.TextInput(c.Key("device-name"), &s.Spotify.DeviceName).Placeholder("MyGoify").Label("填写设备名").FontSize(13).Width(200).Changed() {
					a.keepSettings()
				}
			})
			if a.profile == nil {
				row("未登录", func() {
					if ui.PrimaryButton(c.Key("login"), "登录").Clicked() {
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
					ui.Text(c, "已登录 Spotify").FontSize(12).TextColor(p.muted)
				})
				if ui.Button(c.Key("logout"), "退出登录").Clicked() {
					a.signOut()
				}
			})
		})

		if a.updates {
			var checked string
			if t := lastUpdateCheck(); !t.IsZero() {
				checked = "上次检查：" + relativeTime(t, time.Now())
			}
			group("更新", checked, func() {
				row("自动检查更新", func() {
					on := updateChecks()
					if ui.Switch(c.Key("update-checks"), &on).Label("自动检查更新").Changed() {
						setUpdateChecks(on)
					}
				})
				row("自动下载并安装", func() {
					on := updateDownloads()
					if ui.Switch(c.Key("update-downloads"), &on).Label("自动下载并安装").Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
				row("当前版本 "+mygo.App.Version(), func() {
					if ui.Button(c.Key("check-updates"), "检查更新").Clicked() {
						checkForUpdates()
					}
				})
			})
		}
	})
}
