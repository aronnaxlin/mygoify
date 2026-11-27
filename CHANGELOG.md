# Changelog / 更新日志

## 1.2.0

A mini player, and saved albums.

- Mini player (Linux): click MyGoify's icon to open a small player under it, with the song playing, Like and Save album buttons, playback controls, the volume, and what plays next. Click a song up next to play it, or use Play next to queue it after this one. Click the song's album or artist to open their page in MyGoify. On Omarchy the icon sits in Omarchy's bar as the app's mark, in the bar's color, and the player drops from it in your theme's colors, font and borders. On other desktops the icon is in the tray. The mini player only runs while it is open.
- Media keys and controls with the window closed (Linux): a small background agent runs beside Soloist and shows what plays to the media keys, playerctl and your desktop's media controls. Closing the window now quits the interface, and the music keeps playing; the icon stays to bring it back.
- Saved albums: save an album from its page, and find your saved albums in the sidebar. An album's page also shows whether it is an album, a single or a compilation, how long it plays, and, under its songs, its release date and copyright notices. Spotify's developer API doesn't give play counts, so MyGoify can't show them.
- Instrumentals: songs without lyrics no longer show "No lyrics" or a line saying the song is instrumental. Now Playing moves the cover and controls to the middle instead, and the lyrics stage stays empty.

迷你播放器，以及收藏专辑。

- 迷你播放器（Linux）：点 MyGoify 的图标，在图标下方弹出一个小播放器，有正在播放的歌曲、"喜欢"和"收藏专辑"按钮、播放控制、音量，以及接下来要播放的歌。点"接下来播放"里的一首会直接播放；用"下一首播放"可以把它排到当前这首后面。点歌曲的专辑或歌手，会在 MyGoify 里打开对应页面。在 Omarchy 上，图标是 App 的标志，放在 Omarchy 的栏上，颜色跟随栏；小播放器从图标下方弹出，用你主题的颜色、字体和边框。在其他桌面上，图标在托盘里。小播放器只在打开时运行。
- 关窗口后仍能用媒体键和系统媒体控件（Linux）：一个很小的后台程序和 Soloist 一起运行，把正在播放的内容提供给媒体键、playerctl 和桌面的媒体控件。现在关掉窗口会退出界面，音乐继续播放，图标还在，可以随时打开。
- 收藏专辑：在专辑页收藏专辑，侧栏里可以看到收藏的专辑。专辑页还会显示它是专辑、单曲还是合辑，总时长，以及曲目下方的发行日期和版权信息。Spotify 的开发者接口不提供播放次数，所以 MyGoify 无法显示。
- 纯音乐：没有歌词的歌曲不再显示"暂无歌词"或"纯音乐，请欣赏"。正在播放页会把封面和控件移到中间，歌词舞台保持空白。

## 1.1.0

Recommendations: Spotify's radios, and the playlists Spotify makes for you.

- Radio: start a song's radio from its menu, which begins with that song, or an artist's radio from their page. The home page has a Radio row made from the artists and songs you listen to most. Radios play through Soloist, even while Spotify is limiting the app's requests.
- Made for you: the home page shows the playlists Spotify made for you, such as Discover Weekly, Daily Mixes, the daylist and artist radios, with their own names and covers. They show up as you play them anywhere, or when you paste one's link into the search field. Spotify's developer API doesn't list them, so MyGoify finds them this way.
- Links: paste a Spotify link to an album, artist or playlist into the search field to open it.
- Sign-in storage: Settings › Account can keep your sign-in in local files only your account can read, instead of the system keychain. On macOS, new versions then no longer ask for your password. Switching keeps you signed in.
- Device name: Soloist now shows in Spotify Connect as "MyGoify (your computer's name)", so two computers running MyGoify can be told apart. Renaming keeps the pairing.
- An account without a profile picture shows its initial, as Spotify's apps do, rather than a gray circle.

推荐：Spotify 的电台，以及 Spotify 为你生成的歌单。

- 电台：在歌曲菜单里选"开始电台"，从这首歌开始播；在艺人页点"电台"，播这位艺人的电台。首页新增"电台"一栏，用你最常听的艺人和歌曲当种子。电台经 Soloist 播放，Spotify 限流时也能开。
- 为你推荐：首页显示 Spotify 为你生成的歌单，如 Discover Weekly、Daily Mix、daylist、艺人电台，带官方的名字和封面。在任何设备上播放过的，或者把分享链接贴进搜索框的，都会出现在这里。Spotify 的开发者接口不提供这些歌单，所以 MyGoify 用这种方式找到它们。
- 链接：把专辑、艺人或歌单的 Spotify 链接贴进搜索框，可以直接打开。
- 登录信息存放位置："设置 › 账户"里可以选择把登录信息存在只有你的账号能读的本地文件里，不用系统钥匙串；这样在 macOS 上新版本不再要求输入密码。切换后不用重新登录。
- 设备名：Soloist 在 Spotify Connect 里显示为"MyGoify (你的电脑名)"，两台电脑都运行 MyGoify 时能分清。改名后不用重新配对。
- 没有头像的账号显示名字首字母，和 Spotify 官方客户端一样，不再是灰色圆圈。

## 1.0.1

- Lyrics show their translations only when the interface is in Chinese: the translations are Chinese, and showed under every line in English or Japanese too.

- 只有界面是中文时才显示歌词翻译：翻译只有中文，之前在英文、日文界面下也会显示在每一行下面。

## 1.0.0

The first release of MyGoify: a native Spotify desktop client written in Go with MyGo, which plays through Spotify's official Soloist.

- Sign-in and playback: an onboarding guide walks you through creating a developer app, signing in, downloading Soloist, entering its API key and pairing with your phone. Soloist runs as a background service, so music plays on with the window closed; on quit, choose to stop, pause or keep playing.
- Library and playlists: home, recently played, top artists and tracks, your playlists (create, rename, add and remove tracks), albums and artists. Liked Songs load a page at a time, smoothly even with thousands of songs.
- Search: tracks, albums, artists and playlists, with search history.
- Lyrics: word-by-word timing and translations, in the Now Playing panel and the lyrics stage; third-party lyric sources can be switched off in Settings.
- Audio quality: set MyGoify to Lossless from the device menu of the official Spotify app.
- Visualizer: bars and backdrop move with what Soloist actually plays (PipeWire on Linux, a process tap on macOS).
- Desktop integration: media keys, MPRIS / Now Playing, tray, a single instance, and in-app updates.
- Languages: English, 简体中文, 繁體中文 and 日本語, following the system by default.
- Platforms: Linux (x86_64 / arm64) and Apple Silicon Macs. The Mac app isn't notarized: allow it once in Privacy & Security on first launch (see README).

MyGoify 的第一个正式版本：用 Go 和 MyGo 写的原生 Spotify 桌面客户端，播放交给 Spotify 官方的 Soloist。

- 登录与播放：引导页一步步带你创建开发者应用、登录、下载 Soloist、填写 API key、用手机配对；Soloist 作为后台服务运行，关掉窗口也能接着放，退出时可选停止、暂停或继续播放。
- 曲库与歌单：首页、最近播放、常听的艺人和歌曲、自己的歌单（可新建、改名、增删曲目）、专辑页、艺人页；喜欢的歌曲按页加载，几千首也不卡。
- 搜索：曲目、专辑、艺人、歌单，带搜索历史。
- 歌词：逐字时间和翻译，正在播放页的歌词面板和律动舞台；可在设置里关掉第三方歌词源。
- 音质：可在 Spotify 官方 App 的设备菜单里把 MyGoify 设为无损。
- 可视化：音柱和背景随 Soloist 的真实声音起伏（Linux 用 PipeWire，macOS 用 process tap）。
- 桌面集成：媒体键、MPRIS / Now Playing、托盘、只开一个实例、应用内自动更新。
- 界面语言：English、简体中文、繁體中文、日本語，默认跟随系统。
- 平台：Linux（x86_64 / arm64）和 Apple 芯片的 Mac。Mac 版未经公证，首次打开需在"隐私与安全性"里放行一次（见 README）。

## 0.0.2

- A test of in-app updates: 0.0.1 updates itself to this version.

- 测试应用内更新：从 0.0.1 自动升级到这个版本。

## 0.0.1

- Forked from [MyGo Cloud Music](https://github.com/egoist/mygo-cloudmusic) 0.1.22 and renamed MyGoify, to become a Spotify client. For earlier history, see the upstream repository.

- 从 [MyGo 云音乐](https://github.com/egoist/mygo-cloudmusic) 0.1.22 分叉，改名为 MyGoify，开始做 Spotify 版本。此前的更新历史见上游仓库。
