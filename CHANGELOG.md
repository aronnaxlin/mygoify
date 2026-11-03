# Changelog / 更新日志

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
