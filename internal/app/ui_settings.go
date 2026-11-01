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
		group := func(title string, rows func()) {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(rows)
		}
		row := func(label, desc string, control func()) {
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, label).FontSize(13.5).FontWeight(500)
					if desc != "" {
						ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4)
					}
				})
				control()
			})
		}

		// Only on macOS: elsewhere the icon is always in the notification
		// area.
		if runtime.GOOS == "darwin" {
			group("通用", func() {
				row("应用图标", "显示在菜单栏时，关闭窗口后可从菜单栏图标打开", func() {
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
		// Soloist plays apart from the window.
		group("播放", func() {
			mode := quitModes[0]
			for _, m := range quitModes {
				if m.id == s.Quit {
					mode = m
				}
			}
			row("退出 App 时", mode.desc, func() {
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
			row("单击歌曲即播放", "关闭时，和 Spotify 官方客户端一样双击播放", func() {
				if ui.Switch(c.Key("click-plays"), &s.ClickPlays).Label("单击歌曲即播放").Changed() {
					a.changed()
				}
			})
			if a.canHide() {
				row("关闭窗口时退出", "关闭后，按上面的设置处理音乐；不勾选则只隐藏窗口，音乐继续播放", func() {
					if ui.Switch(c.Key("close-quits"), &s.CloseQuits).Label("关闭窗口时退出").Changed() {
						a.changed()
					}
				})
			}
			row("登录电脑时启动 Soloist", "不打开 App，也能在手机的 Spotify Connect 里选这台电脑", func() {
				if ui.Switch(c.Key("soloist-at-login"), &s.Spotify.SoloistAtLogin).Label("登录电脑时启动 Soloist").Changed() {
					a.changed()
					a.changeAtLogin()
				}
			})
		})

		group("歌词", func() {
			row("歌词服务器", "从这个 LRCLIB 地址获取歌词；lrclib.net 连不上时，可换成镜像或自建的", func() {
				if ui.TextInput(c.Key("lyrics-server"), &s.LyricsServer).Placeholder("https://lrclib.net").Label("歌词服务器").FontSize(13).Width(200).Changed() {
					a.changed()
				}
			})
			row("网易云音乐", "也从网易云音乐找：有逐字时间和翻译，中日文歌更全。这是它的桌面客户端接口，不是公开接口，可能失效", func() {
				if ui.Switch(c.Key("lyrics-netease"), &s.LyricsNetEase).Label("网易云音乐").Changed() {
					a.lyricsSourceChanged()
				}
			})
			row("QQ 音乐", "也从 QQ 音乐找：有逐行歌词和翻译。这是它的网页接口，不是公开接口，可能失效；逐字歌词加密，用不了", func() {
				if ui.Switch(c.Key("lyrics-qq"), &s.LyricsQQ).Label("QQ 音乐").Changed() {
					a.lyricsSourceChanged()
				}
			})
		})

		// Only on macOS, the one with a notch player.
		if a.notch != nil {
			group("刘海播放器", func() {
				row("显示刘海播放器", "在屏幕顶部的刘海处显示迷你播放器，鼠标悬停即可展开", func() {
					if ui.Switch(c.Key("notch"), &s.Notch).Label("显示刘海播放器").Changed() {
						a.changed()
					}
				})
				row("播放时显示实时活动", "在刘海两侧显示封面与音频波形，可能遮挡刘海旁的菜单栏图标", func() {
					if ui.Switch(c.Key("activity"), &s.NotchActivity).Label("播放时显示实时活动").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
				row("在无刘海的屏幕上常驻显示", "关闭时，将鼠标移至屏幕顶部中央即可唤出，不会遮挡菜单栏", func() {
					if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label("在无刘海的屏幕上常驻显示").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
			})
		}

		group("外观", func() {
			row("外观", "", func() {
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

		group("实验性功能", func() {
			row("进度条烟雾特效", "播放时进度条化作拖着烟雾的光点，随音乐起伏；会多用一些电量", func() {
				if ui.Switch(c.Key("smoke"), &s.Smoke).Label("进度条烟雾特效").Changed() {
					a.changed()
				}
			})
			row("烟雾背景", "播放页的背景化作封面色调的烟雾，缓缓升起，随低音起伏；仍在打磨，会多用一些电量", func() {
				if ui.Switch(c.Key("fog"), &s.Fog).Label("烟雾背景").Changed() {
					a.changed()
				}
			})
		})

		group("账号", func() {
			row("连接 Spotify", "创建应用、登录、Soloist、API key 和配对，每一步可以单独重做", func() {
				if ui.Button(c.Key("onboarding"), "设置…").Clicked() {
					a.showMain()
					a.openOnboarding()
					a.win.Invalidate()
				}
			})
			if a.profile == nil {
				row("未登录", "登录后，每日推荐与歌单将根据你的口味生成", func() {
					if ui.PrimaryButton(c.Key("login"), "扫码登录").Clicked() {
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
			group("更新", func() {
				desc := "每天检查一次是否有新版本"
				if t := lastUpdateCheck(); !t.IsZero() {
					desc += "；上次检查：" + relativeTime(t, time.Now())
				}
				row("自动检查更新", desc, func() {
					on := updateChecks()
					if ui.Switch(c.Key("update-checks"), &on).Label("自动检查更新").Changed() {
						setUpdateChecks(on)
					}
				})
				row("自动下载并安装", "在后台下载新版本，下次打开时生效", func() {
					on := updateDownloads()
					if ui.Switch(c.Key("update-downloads"), &on).Label("自动下载并安装").Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
				row("当前版本 "+mygo.App.Version(), "", func() {
					if ui.Button(c.Key("check-updates"), "检查更新").Clicked() {
						checkForUpdates()
					}
				})
			})
		}
	})
}
