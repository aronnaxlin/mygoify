# MyGoify

用 Go 和 [MyGo](https://github.com/egoist/mygo) 写的第三方 Spotify 桌面客户端，原生界面，无 WebView，低内存。主要支持 Omarchy（Hyprland）和 Apple Silicon 的 macOS。

> **开发中。** 现在仓库里还是 [MyGo 云音乐](https://github.com/egoist/mygo-cloudmusic) 的代码，正在按 [ROADMAP](ROADMAP.md) 改造。

## 怎么工作

- **播放**：Spotify 官方的无头播放器 [Soloist](https://github.com/spotify/soloist)，作为后台服务运行；关掉窗口音乐照放。
- **数据**：Spotify 官方 Web API，用你自己的开发者应用登录。
- **界面**：沿用 MyGo 云音乐。

设计细节见 [docs/DESIGN.md](docs/DESIGN.md)。第一版只做音乐：播放、曲库、歌单、搜索、歌词，不做播客。

使用需要 Spotify Premium 和一个 Spotify for Developers 账号。

## 凭据放在哪

- Spotify 登录和 Soloist 的 API key 存在系统钥匙串里（macOS 钥匙串，Linux 的 Secret Service，比如 GNOME Keyring、KeePassXC）；client ID 等其余设置存在设置文件里。
- **Soloist 只能从命令行参数拿到 API key**，没有别的方式。所以 Soloist 运行时，同一台电脑上的其他用户用 `ps` 能看到这个 key。服务的配置文件里不会写 key，只在启动时从只有你能读的文件里读进来。单用户电脑上问题不大；多人共用的机器上，Linux 可以用 `hidepid=2` 重新挂载 `/proc` 来遮住别人的进程参数。

## 构建

需要 Go 1.27+。

```bash
go tool mygo dev     # 开发模式运行
go tool mygo build   # 打包到 build/
go test ./...        # 运行测试
```

## 致谢

- [MyGo 云音乐](https://github.com/egoist/mygo-cloudmusic)：本项目从它分叉，界面和交互都来自它
- [MyGo](https://github.com/egoist/mygo)：原生 UI 与应用框架
- [Omarchy-Spotify](https://github.com/stappmus/Omarchy-Spotify)、[go-librespot](https://github.com/devgianlu/go-librespot)、[spotify-player](https://github.com/aome510/spotify-player)：实现经验的参考

本项目与 Spotify 官方无关。Spotify 是 Spotify AB 的商标。音乐版权归各权利人所有。

## 许可证

[AGPL-3.0](LICENSE)
