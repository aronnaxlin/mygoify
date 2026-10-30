# M0 PoC 记录

ROADMAP M0 各项验证的结果。代码在 `cmd/poc`。

环境：Omarchy（Linux 7.2.8，x86_64，PipeWire），Soloist 1.3.10.31（构建于 2026-10-30，89 天后过期），Spotify Premium 账号，2026-10-30。

## 结果

| # | 内容 | 结果 |
|---|---|---|
| P1 | 运行 Soloist，`--ws 127.0.0.1:0`，连上 WebSocket | **通过**。地址和端口从 data-dir 下的 `ws.addr` / `ws.port` 读 |
| P2 | 手机配对后，重启会话仍在 | **通过**。重启后自动恢复登录，但不是当前播放设备（`is_active: false`） |
| P3 | Web API PKCE，用自己的 client ID 登录 | **通过**。DESIGN §5.1 的 11 个 scope 全部授予；`/me/playlists`、`/playlists/{id}/items`、`/search` 可用 |
| P4 | `PUT /me/player/play`（`context_uri` + `offset` + `device_id`）让 Soloist 从歌单第 N 首开始播 | **通过**，确认有声音。Web API 0.5 s 接受，约 2.0 s 后 Soloist 推送 `playing`，context 是整个歌单。Soloist 不是当前设备时也能直接用 `device_id` 播 |
| P5 | 不用手机配对 | **推迟**。Soloist 的 zeroconf `tokenType` 是 `default`，不是 access token，要先有 librespot 那样的可复用凭据才能交给它 |
| P6 | 进度插值的精度 | **基本通过**。连续播放 60 s，61 次进度推送：相邻推送之间外推的偏差 ≤ 1 ms；`timestamp_ms` 与本机时钟之差中位数 0.8 ms（最大 40.6 ms，为送达延迟），可直接用本机时钟插值。到耳朵的音频输出延迟没有测，M5 做歌词时用耳朵核对，必要时加固定补偿 |
| P7 | 资源占用 | **通过**。空闲且已登录：PSS ≈ **56 MB**（主进程 42 + crashpad 辅助进程 14）。播放时：PSS **62.5 MB**，60 s 内不增长；CPU **0.85%** 单核。PipeWire 输出流名 `spotify`，44.1 kHz。响度均衡和 seek 延迟未测 |
| P8 | Apple Silicon 的 Mac | 待测，需要 Mac |
| P9 | API key 不出现在 `ps` 里 | **不通过**。Soloist 只接受 `-k` 参数，不读环境变量，key 会出现在 `/proc/<pid>/cmdline` 里，同机其他用户可见 |

## 其他发现

- **缓存目录**：`-C` / `--cache-dir` 被忽略，缓存实际在 `<data-dir>/cache`。缓存默认不限大小，要用 `-z` 设上限。
- **官方配对模式**：`soloist --pair` 只做 Connect 配对然后退出，可以用在引导页。
- **`soloist ctl`**：自带 `status`、`now --json`、`trace`（输出所有 WebSocket 事件），调试时很方便。
- **从手机交接**：手机把正在暂停的歌交给 Soloist 时，Soloist 也是暂停状态；中间会短暂出现一次 `idle` 和空的 context，界面要能容忍这种抖动。
- **事件格式**：曲名在 `item.decorations.identity.name`，艺人在 `item.decorations.creators[].entity.decorations.identity.name`，时长在 `item.decorations.playback.duration_ms`，与官方文档一致。
- **搜索**：`limit` 最大 10，传 11 返回 `400 Invalid limit`；结果里的 `total` 恒为 0，界面不能显示"共 N 条"，翻页以 `next` 是否为空为准。
- **Web API 设备列表**：除了 Soloist，还会列出这台机器上的其他客户端，所以要按设备名匹配，不能取第一个。

## 对设计的影响

- **P9**：正式版里，Soloist 用 systemd user unit 启动，key 放在只有当前用户可读的 `EnvironmentFile` 里，命令行写 `-k ${SOLOIST_API_KEY}`。这样 unit 文件里不出现明文，但进程命令行里仍然有。Omarchy 默认没有用 `hidepid` 挂载 `/proc`，单用户机器上可以接受，在 README 里写明。
- **P2**：App 发播放命令一律带上 `device_id`，不依赖 Soloist 是不是当前设备。
