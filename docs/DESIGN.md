# mygo-spotify 设计文档

> 状态：草案 v0.1（2026-10-30）。App 名称已定为 **MyGoify**（仓库 `aronnaxlin/mygoify`）
> 范围：第一版（v1.0）只做音乐：播放、歌单、搜索、曲库、歌词。不做播客、有声书。

## 1. 目标与非目标

**目标**

- 一个原生界面的第三方 Spotify 桌面客户端，界面和交互沿用 [mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic)，用 [MyGo](https://github.com/egoist/mygo) 原生 UI，不用 WebView。
- 低内存：Linux 上窗口打开时界面进程 PSS ≤ 200 MB；只在后台播放时，常驻内存只有播放引擎（约 60 MB）。对照：官方桌面客户端约 912 MB。
- 平台：**Omarchy（Hyprland / Wayland）为主**，**Apple Silicon 的 macOS** 为辅。
- 合规：只用 Spotify 官方提供的播放引擎和公开 API，不碰 DRM。

**非目标（v1 不做）**

- 播客、有声书、评论、动态、用户主页。
- 下载、导出、离线缓存音频文件。**永远不做**，见 §9。
- 自己解码和播放音频。v1 不做，go-librespot 只作为 v1 之后的备用后端，见 §4.4。
- Intel Mac。Soloist 没有 Intel Mac 版本。
- Windows。Soloist 和 MyGo 都支持 Windows，但 v1 不测试、不发布。

## 2. 背景：为什么是这套架构

调研结论（详见对话记录，以下只记结论）：

| 方案 | 结论 |
|---|---|
| go-librespot 放进同一进程播放 | **不选作主力**。2025 年底起，新账号和部分曲目取不到旧的 AES 音频密钥（[go-librespot #279](https://github.com/devgianlu/go-librespot/issues/279)、[librespot #1649](https://github.com/librespot-org/librespot/issues/1649)，2026-10 仍未解决）。唯一的修法是 PlayPlay DRM，有 DMCA 风险。 |
| **Spotify Soloist**（2026-08 官方发布） | **选为播放后端**。官方无头 Connect 播放器，和官方 App 同一个引擎，支持无损，Linux、Apple Silicon Mac、Windows 都有版本，带本地 WebSocket API。 |
| 官方 Web API（开发模式） | **选为数据层**。2026 年 2 月起限制很多（§5.3），但搜索、曲库、歌单、播放控制都还能用。 |
| Omarchy-Spotify（QML 插件） | 作为参考，借鉴它的限流、授权、Connect 处理经验。它只支持 Omarchy；我们的定位是跨平台的独立 App。 |

## 3. 总体架构

```
┌──────────────────────────── mygo-spotify（Go，单个二进制）───────────────────────────┐
│  internal/app       界面（沿用 cloudmusic）：页面、播放条、正在播放、歌词、设置        │
│        │ 只依赖下面两个接口                                                           │
│  internal/library   数据：Web API 客户端 + 限流 + 缓存 + 模型                          │
│  internal/playback  播放：Backend 接口 ── soloist 实现（v1）/ librespot 实现（v1 之后） │
│  internal/lyrics    歌词：解析器沿用 cloudmusic；数据源 LRCLIB                          │
│  internal/auth      PKCE 登录、token 刷新、凭据存进系统钥匙串                           │
└───────────────┬──────────────────────────────────────┬────────────────────────────┘
                │ HTTPS（Web API）                     │ WebSocket ws://127.0.0.1:<port>
                ▼                                      ▼
        api.spotify.com                       soloist（官方二进制，独立进程，用户级服务）
                                                       │ PipeWire / CoreAudio
                                                       ▼
                                                      声卡
```

**关键原则**

1. **播放和界面分属两个进程。** Soloist 常驻，界面随用随开。关掉界面，音乐继续放。这是低内存的来源。
2. **界面不直接碰 Web API 或 WebSocket**，只通过 `library` 和 `playback` 两个接口。测试时替换这两个接口，沿用 cloudmusic 用包级变量替换网络的测试写法。
3. **播放状态以 Soloist 为准。** 队列、进度、随机和循环都在 Soloist 那边，界面只做显示和发命令，自己不维护队列。这一点和 cloudmusic 不同。

## 4. 播放层（internal/playback）

### 4.1 接口

```go
type Backend interface {
	// PlayContext 从 context（专辑、歌单、艺人）里 offset 指定的那首开始播。
	// offset 为空时从头播。
	PlayContext(ctx context.Context, contextURI, offsetURI string) error
	// PlayTracks 播放一组曲目（筛选、排序后的列表；Web API 一次最多 100 首）。
	PlayTracks(ctx context.Context, uris []string, offset int) error
	Resume() error
	Pause() error
	Next() error
	Previous() error
	Seek(pos time.Duration) error
	SetVolume(v float64) error // 0–1
	SetShuffle(on bool) error
	SetRepeat(mode RepeatMode) error
	AddToQueue(uri string) error

	// State 返回最新状态；Changed 在状态变化时（主线程上）通知界面。
	State() State
	Queue() Queue
	Changed(func())
}
```

`State` 里的 `Position` 由 Soloist 推送的 `position_ms` + `timestamp_ms` + `speed` 在本地插值，歌词和进度条都读它，不轮询。

### 4.2 Soloist 实现

**运行时配置（硬约束）**

Soloist 的版本每 90 天过期；API key 官方没说会过期，但社区有"90 天"的说法，按会失效设计。所以**和 Soloist、Spotify 账户相关的东西一律是运行时状态，换掉任何一项都不需要 MyGoify 发新版**：

| 配置 | 存在哪 | 谁写入 | 失效时 |
|---|---|---|---|
| Soloist 可执行文件路径 | 设置文件（用户数据目录） | 引导页；设置页可换 | 退出码 `10`（版本过期）→ 只走"更新 Soloist"这一步：选新文件或打开官方下载页，换完重启服务 |
| Soloist API key | 系统钥匙串 | 引导页；设置页可换 | Soloist 用它登录失败 → 只走"重新填 key"这一步，其余配置不动 |
| 设备名 | 设置文件 | 默认 "MyGoify"，设置页可改 | — |
| Web API client ID、回调地址 | 设置文件 | 引导页 | 刷新 token 被拒（`auth.ErrRevoked`）→ 只走"重新登录"这一步 |
| Web API refresh token | 系统钥匙串 | 登录时 | 同上 |

- **不编译进安装包的**：以上全部，以及 Soloist 二进制本身（条款也禁止再分发）。安装包里没有任何账户相关的值。
- **提前提醒**：App 启动 Soloist 前运行 `soloist --version`，读出构建时间戳，到期日 = 构建日 + 90 天。设置页显示到期日，到期前 14 天开始提醒更新。
- **哪一项失效就只修哪一项**：引导页的每一步可以单独重做，不会让用户从头配置。
- **未经验证的 key 不能碰真正的配对数据**（2026-10-31 实测，见 POC.md）：key 无效时 Soloist 不退出，保持"未登录"，同时**删除 data-dir 里保存的配对会话**，之后即使换回正确的 key 也要用手机重新配对。所以填入或更换 key 时，App 先停掉 Soloist 服务，把 data-dir 复制一份到临时目录（不带 `ws.*`、锁和 crashpad），用新 key、另一个设备名和单独的缓存目录在副本上启动 Soloist（`Service.ChangeKey` → `soloist.CheckKey`）：WebSocket 的 `auth_state` 在 30 秒内变成 `logged_in: true` 才算有效，再存进钥匙串，用新 key 重启服务；否则提示 key 无效，用原来的 key 重启，原来的配对不受影响。副本必须带上 `.device_id`：配对只对配对时的设备 ID 有效，2026-10-31 实测不带它副本会当作新设备等配对，带上则 1.5 秒内恢复会话。副本因此和真服务是同一台设备，不能同时在线，所以验证期间服务要停，播放会中断这几秒；换 key 很少见，可以接受。首次配对时还没有会话可试，key 填错的表现在引导页实测。
- **待实测**：换新版本后 `--data-dir` 里的配对会话是否保留。现在的版本就是最新的，要等 Spotify 发布下一个版本；在此之前按"会丢，需要重新配对"设计。

**进程管理**
- Soloist 作为**用户级服务**运行：Linux 用 systemd user unit，macOS 用 launchd agent。由 App 安装，App 启动时拉起，这一点参考 Omarchy-Spotify。
- Soloist 和界面分开运行，所以退出 App 后怎么处理音乐，由用户在设置页"播放"里选择：
  - **退出 App 时**：默认"停止并关闭 Soloist"，先暂停，等 Soloist 回报已暂停，再停掉服务，从手机投放到这台电脑的播放也随之停止；"暂停"只暂停，Soloist 留在后台（约 60 MB），下次打开更快；"继续播放"什么都不做，退出后要回到 App 才能控制（直到 v1.1 的 agent，见 §4.3）。暂停只针对这台电脑正在播的内容，别的设备上的播放不动。引导页里点"重新启动"时不做这些，Soloist 照常播放。
  - **关闭窗口时退出**：默认关窗只隐藏窗口；打开后关窗就按上一项退出。没有托盘的 Linux 桌面上本来就是关窗即退出，不显示这一项。
  - **登录电脑时启动 Soloist**：默认关。打开后 launchd 的 `RunAtLoad`、systemd 的 `enable` 让 Soloist 在登录时启动，不开 App 也能在 Spotify Connect 里选这台电脑。切换这一项只改服务文件，不重启正在播放的 Soloist。
- 启动参数：`--device-name`、`--api-key`、`--data-dir`、`--cache-dir`、`--ws 127.0.0.1:0`。端口用 0 让系统分配，实际地址从 data-dir 里的 `ws.addr` / `ws.port` 读。
- API key 和 Spotify 登录存在系统钥匙串里（`internal/secret`）。mygo 没有凭据存储，也不用 cgo，所以用 purego 调系统接口：macOS 调 Security framework，不用 `security` 命令，因为它要把密码放进参数；Linux 用 godbus 调会话总线上的 Secret Service（GNOME Keyring、KeePassXC 等）。没有钥匙串时退回只有当前用户可读的文件；以前存在文件里的值第一次读取时搬进钥匙串。开发时设 `MYGOIFY_SECRETS=files` 只用文件：macOS 每遇到一个新构建都会弹窗问能不能读钥匙串。
- PoC 证实 Soloist 只接受 `-k` 参数（P9）：App 每次启动服务前，从钥匙串把 key 写进只有当前用户可读的文件。systemd unit 用 `EnvironmentFile` 读它，写 `-k ${SOLOIST_API_KEY}`；launchd agent 用 `sh -c` 读它。unit 和 agent 文件都不含 key，但 Soloist 运行后，它的命令行里仍然有 key，同机其他用户用 `ps` 能看到，这一点 README 里写明。
- 签名和钥匙串：macOS 按签名决定哪个程序能读钥匙串条目。用 Developer ID 签名，升级后不弹窗；没有证书时用 ad-hoc 签名，每个新版本第一次读钥匙串会弹一次授权。
- **获取 Soloist：** Spotify 不允许再分发二进制。引导页提供"从 Spotify 官方下载"按钮（§10 问题 3）：按平台选 `soloist_release_<arch>.tar.gz`，下载前说明来源并让用户确认，解压到 App 的数据目录，用 `--version` 检查能运行。也可以选择已下载的文件。更新走同一个流程。App 安装包里不带 Soloist。

**命令分工**（根据 WebSocket API 的能力决定）

| 操作 | 通道 | 原因 |
|---|---|---|
| 从头播放一个 context（专辑、歌单的播放按钮） | WebSocket `play`（`uri` 为 context） | 从第一首能播的开始，不需要 offset；随机播放打开时 Soloist 会另选一首，仍走 Web API |
| 播放 context 里的某一首 | Web API `PUT /me/player/play`（`context_uri` + `offset` + `device_id`） | WebSocket 的 `play` 只能传一个 URI，不能指定 offset |
| 播放一组曲目 | Web API `PUT /me/player/play`（`uris`，最多 100 首） | 同上 |
| 暂停、继续、上一首、下一首、seek、音量、随机、循环、加入队列 | WebSocket | 本地通道，不消耗 Web API 配额，延迟低 |
| 状态、进度、队列 | WebSocket 事件（`playback_state`、`position_sync`、`queue_changed`） | 推送，不轮询 |

`device_id` 来自 `GET /me/player/devices`，按设备名匹配 Soloist，找到后存进数据目录的 `soloist-device`，以后启动不再查；Web API 回 404 时再查一次。Spotify 限流期间，这两种播放直接失败，播放条显示"Spotify 限流中，HH:MM 后再试"；从头播放和 WebSocket 的其他命令照常可用。不退回本地播放：只播点中的那首、再把后面的加入队列，会让其他设备显示的播放来源变成单曲，加入的歌也会一直留在 Spotify 的队列里，排在用户之后播的内容前面；连续下一首跳过去，又会给播放记录留下一串跳过。

App 空闲时不调 Web API：窗口隐藏或静止、歌曲自然切换，都不发请求（`TestIdleAsksNothing`）。只有用户的操作才会请求，例如打开页面、搜索、喜欢。命令发出后的结果以后续的 `playback_changed` 事件为准；`command_result` 只表示命令已派发。

**首次配对**
- Soloist 要先在 Spotify Connect 里登录一次。默认流程：引导用户在同一局域网的手机 App 里选中这个设备播放一次，之后会话保存在 `--data-dir` 的 `cache/dbrts`，用设备 ID 加密。2026-11-01 实测：这个文件解不开时，Soloist 当作没有会话，也不会用新登录覆盖它（"to avoid data loss"），于是每次启动都要从手机重新连；把它移走、连一次就好了。怎么坏的没复现，所以停服务要等 launchd 真正放手（`Service.Stop`），再动它的数据或重新启动它：`launchctl bootout` 返回时还没有，紧接着 bootstrap 会失败。
- 待验证的改进：App 自己当 zeroconf 发送方，把 Web API 的 access token 交给 Soloist 完成配对，不需要手机（Omarchy-Spotify 对 access-token 类型的接收端就是这样做的）。见 PoC P5（已推迟）。

**WebSocket 安全**
- WebSocket 没有任何认证，只绑定 `127.0.0.1`。
- 连接时校验 data-dir 里的 `ws.port`，并确认端口属于我们拉起的那个 Soloist 进程。

### 4.3 后台控制（v1.1）

界面关掉以后，媒体键、Waybar、`playerctl` 需要 MPRIS 才能控制，但 Soloist 本身不提供 MPRIS。

- v1：关窗口默认**缩到托盘**，界面进程仍在，MPRIS 由 mygo 的 Now Playing 提供。代价是常驻约 190 MB。用户也可以选择关窗或退出后让音乐继续（§4.2），这时没有媒体键。
- v1.1：加一个极小的 `mygo-spotify-agent`（纯 Go、无界面）。它连 Soloist 的 WebSocket，导出 MPRIS（macOS 上是 Now Playing）。之后关窗口就让界面进程直接退出，常驻目标 ≤ 80 MB（Soloist + agent）。

### 4.4 备用后端（v1 之后）

把 go-librespot 裁剪成纯 Go：切掉 `session → events/impl → player` 引入的 cgo 解码器，用 mygo 的 `audio/vorbis` 解码。实现同一个 `Backend` 接口，给 Intel Mac、不想装 Soloist 的用户用，明确标注"部分账号或曲目可能无法播放"。

### 4.5 音频可视化（internal/levels）

Soloist 不提供音频数据，播放条的烟雾光点、歌曲行和刘海里的音柱、烟雾背景要的频谱，从系统混音时听到的 Soloist 的声音里算。

- **macOS 14.2+**：Core Audio 的 process tap 只听 Soloist 这一个进程（按 data-dir 里的 `soloist.pid` 找），放进一个只有它的私有聚合设备，IO 回调拿到 48 kHz 浮点立体声。全部用 purego，不用 cgo。tap 设为 private、不静音，Soloist 照常从原来的输出设备出声。首次会弹"录制系统音频"授权（`NSAudioCaptureUsageDescription`，在 mygo.json 里）；不给授权时 tap 只收到静音，界面退回空闲动画。`go tool mygo dev` 从终端启动时，授权记在终端名下，终端已有授权就不再弹。
- **何时开**：Soloist 第一次报 `playing` 时建 tap，连接断开（Soloist 重启，换了进程）时拆掉，下次播放再按新 pid 建。暂停时系统不再混 Soloist 的声音，回调停止，不耗电。刚启动时 Core Audio 可能还不认识这个进程，隔 2 秒重试，最多 3 次。Linux 上 tap 一直跟着这个进程，它的流没了就等下一个。
- **怎么算**：只保留最近约 0.7 秒的单声道样本（环形缓冲，不断覆盖），界面要时取 1024 点加 Hann 窗做 FFT，按音高从 40 Hz 到 14 kHz 均分 24 段（沿用 cloudmusic 的算法）。取的是"现在听到的"那一段：减去默认输出设备的延迟（蓝牙耳机一两百毫秒）。Soloist 的音量和响度均衡会作用在听到的声音上（2026-11-01 实测，音量 25% 时 RMS 只有约 1/40），所以按最近最响的一段做自动增益：它立即跟上，每秒回落 4 dB，最低 −70 dB。
- **红线**：样本只在内存里，用来算 24 个数，不写盘、不发送，也不能导出（§9）。
- **Linux（PipeWire）**：用 PipeWire 自带的两个工具，不用 cgo，也不自己实现 PipeWire 协议。`pw-dump --monitor` 盯着图的变化，找 Soloist 进程（`soloist.pid`，对 client 的 `application.process.id`）放出的 `Stream/Output/Audio` 节点，不按名字找，因为官方客户端的流也叫 `spotify`；找到后起一个 `pw-record --raw --format f32 --target <节点 serial>`，直接录这个流的输出（混音之前，采样率跟流一致，44.1 kHz），经管道交给 meter。录音节点设为 `node.passive`（暂停后不让输出设备一直醒着）、`node.dont-fallback` 和 `node.dont-reconnect`（目标不在或消失时直接退出，不会改录麦克风）。Soloist 暂停时没有流，再播放时开新流，tap 跟着换节点，不必重建。延迟取流的输出端口上 PipeWire 算好的到输出设备的延迟（几个 quantum 加若干样本和纳秒，蓝牙会更长），quantum 按 settings 里的 `clock.quantum` 算。没装这两个工具时 `Tap` 返回 `ErrUnsupported`，界面是空闲动画。录音节点在 PipeWire 里是一个录音流，状态栏的录音指示可能会亮。2026-11-01 实测：自己放的测试音能听到；Soloist 真正播放时，它的 client 的 `application.process.id` 就是 `soloist.pid`，流是 44.1 kHz 的 `spotify` 节点，tap 3 秒内持续算出随音乐变化的频谱，停下后没有遗留的 `pw-record`。

## 5. 数据层（internal/library）

### 5.1 授权（internal/auth）

- Authorization Code + **PKCE**，回调地址是 `http://127.0.0.1:<固定端口>/callback`，需要用户在 Dashboard 里登记。
- **用户自己的 client ID**：用户本来就要注册 Spotify for Developers 来拿 Soloist 的 key，引导页顺便带着建一个应用。这样不和别人共用配额，也避开了共享 ID 被限流的问题。**不内置共享 client ID。**
- 不用 librespot 的内部 token 调 Web API，有人遇到过 `Retry-After: 86400`，也就是要等一整天。
- refresh token 存在系统钥匙串里；access token 只放内存，过期前刷新。
- scope 只申请用到的：`user-read-playback-state user-modify-playback-state user-read-currently-playing user-read-recently-played user-top-read user-library-read user-library-modify playlist-read-private playlist-read-collaborative playlist-modify-private playlist-modify-public`。不申请 email 和 profile。

### 5.2 请求管线

```
调用方 → 去重（相同请求共用一个 pending）→ 缓存（内存 LRU + 磁盘）→ 限流器 → HTTP
```

- **429 处理**：在 `Retry-After` 的基础上加 10–30% 抖动再重试，最多 4 次；body 里 `reason: QUOTA_EXCEEDED` 时**不重试**，界面提示配额已用完。全局冷却期间，所有新请求排队，不并发重试。
- **合并请求**：拖动音量、seek 走 WebSocket，不占 Web API；需要走 Web API 的连续操作合并成一次，间隔不低于 250 ms。
- **缓存**：专辑、艺人、曲目元数据缓存 24 小时；歌单按 `snapshot_id` 判断是否失效；空结果也缓存，避免反复请求。
- **批量接口已删除**：只能逐个按 ID 取，所以并发上限设为 4，能用列表接口里带的字段就不再单独请求。

### 5.3 2026 年开发模式的限制，以及界面怎么应对

依据[官方迁移指南](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide)和 [7 月配额更新](https://developer.spotify.com/blog/2026-07-23-web-api-quota-updates)：

| 限制 | 界面上的处理 |
|---|---|
| 搜索每页最多 10 条 | 每类结果先显示 10 条，"显示更多"用 offset 翻页 |
| 别人的歌单（包括 Daily Mix 等官方生成的歌单）不返回曲目 | 歌单页显示封面、名称、描述和**播放按钮**，曲目区域说明原因；播放走 `context_uri` |
| `artists/{id}/top-tracks` 已删除 | 艺人页的"热门"用 `search?q=artist:"名字"&type=track` 拼出来，并注明来源 |
| `browse/new-releases`、`browse/categories` 已删除 | 首页不做"浏览"区，改成：最近播放、常听艺人和曲目、我的歌单 |
| `GET /users/{id}`、`/users/{id}/playlists` 已删除 | 不做用户主页 |
| `/me` 不返回 `product` | 无法判断是否 Premium，以 Soloist 能不能播为准 |
| 库操作改成 `PUT/DELETE /me/library`，参数是 URI | 喜欢、取消喜欢统一走它 |
| 所有 client ID 共用开发者账号的配额 | 设置页显示最近一次 429 的时间和原因 |

## 6. 界面：从 cloudmusic 迁移

| cloudmusic 文件 | 处理 | 说明 |
|---|---|---|
| `ui_kit` `ui_main` `ui_skeleton` `ui_pages` `menu` `icons` `tray` `updates` `settings` | **保留** | 外壳、组件、导航、托盘、自动更新 |
| `ui_player` `ui_nowplaying` `ui_cover` `backdrop` `artwork` | **保留**，数据源换成 `playback.State` | 播放条、正在播放、封面、背景 |
| `ui_wall` | **保留**，队列来自 Soloist 的 `queue_changed` | 队列封面墙 |
| `ui_stage` `ui_stage_styles` | **保留**（M5） | 歌词舞台 |
| `ui_album` `ui_artist` | **改造** | 艺人页去掉动态，热门改用搜索拼出来 |
| `ui_search` | **改造** | 去掉"用户、播客"两类；按回车才搜，不做输入时实时搜索（省配额） |
| `ui_like` `ui_songmenu` `ui_playlistedit` | **改造** | 歌单只有自己的或协作的才能编辑 |
| `ui_account` | **重写** | 换成引导页：client ID → 登录 → Soloist 下载和 key → 配对 |
| `session` | **简化** | 只保存最后打开的页面；播放进度由 Soloist 自己恢复 |
| `notch_darwin` `ui_notch` | **保留**（仅 macOS） | 刘海播放器 |
| `ui_comments` `ui_quiet` `ui_events` `ui_user` `ui_podcast` `ui_captions` `captions` `ui_radio` `ui_daily` `ui_suggest` `pickup` | **删除** | 没有对应的数据，或者属于播客 |
| `internal/netease` `internal/speech` | **删除** | |
| `internal/player` | **删除**，只把队列显示相关的逻辑挪进 `playback` | 解码、流、缓存、淡入淡出都由 Soloist 负责 |
| `internal/lyrics` | **保留**解析器，换数据源 | |

## 7. 歌词（internal/lyrics）

- 三个源，`lyrics.Sources` 同时问：有逐字时间的那个优先，否则取第一个有逐行时间的，翻译取有翻译的那个。拿到逐字时间和翻译就算不会再好，立刻返回并取消其余的请求；只有逐字或只有逐行时再等 1.5 秒（`grace`），看还在问的源有没有更好的，到点返回当前最好的；一个源都没找到时才等全部答完。没有歌名的曲目（Soloist 刚连上时）不问，也不缓存。任何一个源出错（不是"没有"）而别的源也没找到时，整体算"没问成"：不缓存，App 后台重试。
  - **NetEase**（`lyrics.NetEase`，默认开）：中日文歌最全，有逐字（YRC）和翻译。搜索走 `music.163.com/api/search/get/web`；歌词走桌面客户端的 eapi（AES-128-ECB 签名，key 是每个客户端里都有的公开常量），一次拿到 LRC、翻译和 YRC。境外常回 `code:460`，那就当出错。
  - **LRCLIB**（`lyrics.LRCLIB`，默认开，地址可换）：开放、免 key，只有逐行 `syncedLyrics`，找法：
    1. `/api/get` 用 `track_name + artist_name + album_name + duration` 精确匹配。LRCLIB 库里没有时会去别处找，常常一秒后回 503（2026-11-01 实测）：503 也直接进下一步，不重试。
    2. `/api/search` 按歌名和第一个艺人搜，取时长差 3 秒内最近的；一样近时取同专辑的（原声带、单曲常收同一首）。
    3. 歌名去掉 Spotify 加的后缀（` - Remastered 2011`、` (feat. …)`）再搜一次。
    4. 只按歌名搜，时长差 1 秒内才算：Spotify 可能用 LRCLIB 不用的写法给艺人起名（`Rene Liu` 和 `刘若英`、`Hebe Tien` 和 `田馥甄`）。同名同长基本是同一首，最多是翻唱。
    5. 搜索回 503 或 429 时隔 1、2、3、4 秒重试，最多 5 次。回 400 当作"没有"，缓存下来，不再一次次问。
  - **QQ 音乐**（`lyrics.QQ`，默认关）：中文歌的逐行和翻译。搜索走 `u6.y.qq.com/cgi-bin/musicu.fcg` 的 mobile 搜索，歌词走 `c.y.qq.com` 的 `fcg_query_lyric_new.fcg`（base64，内容是 HTML 转义过的）。逐字是 QRC，加密，不做；对没有版权的歌回 `-1901`，按"没有"处理。
- 后两个按名字加第一个艺人搜，取时长差 3 秒内最近的，一样近时取同专辑的；名字带 Spotify 后缀时去掉再搜一次。
- 命中率（2026-11-01，随机抽 50 首已点赞歌曲，三源并发，每首 15 秒超时）：

  | 源 | 找到 | 没有 | 出错 | 中日文歌 | 中位耗时 |
  |---|---|---|---|---|---|
  | NetEase | 48 | 2 | 0 | 20/21 | 0.3 秒（p90 0.4） |
  | LRCLIB | 20 | 2 | 28 | 6/21 | 15 秒（p90 15） |
  | QQ 音乐 | 47 | 3 | 0 | 20/21 | 0.5 秒（p90 0.6） |

  - 有一个源找到的：49/50。LRCLIB 没找到或出错的 30 首里，NetEase 找到 28 首，QQ 找到 29 首。
  - NetEase 找到的 48 首里，32 首有逐字，28 首有翻译。QQ 这 50 首里没有一首带翻译。
  - LRCLIB 当晚大量超时（15 秒放弃），比 2026-11-01 早先那次的 32/7/11 更差；NetEase 在它的位置上答得又快又全，实际顺序里 LRCLIB 常常没被问到。
- 非公开接口的风险：网易云和 QQ 的接口不是公开 API（§9 只禁 Spotify 的 color-lyrics 和 `sp_dc`）。它们可能改、可能限流、可能按地区拒绝，所以各有一个开关，默认只开网易云。
- App 这边兜底（`internal/app/lyrics.go`）：
  - 播一首时顺便取播放顺序里的下一首，存进磁盘缓存；同一首同时只问一次，下一首开播时还在取，就等那次的结果。
  - 正在播的这首问不到时，隔 30 秒、1 分钟、2 分钟在后台再问，换歌就停。
  - 骨架屏最多显示 4 秒，之后显示"暂无歌词"；晚到的歌词照样换上。
  - 设置里可以换 LRCLIB 地址（镜像或自建），默认 lrclib.net。
- 缓存：按曲目 ID 存到缓存目录下的 `mygoify/lyrics`，记 `At`、`LRC`、`Translation`、`YRC`、`Source` 和 `Asked`（写的时候启用了哪些源）。找到的一直留着；"没有"存 7 天，`Asked` 和当前开关不一致时立即失效，所以打开一个新源后旧的"没有"不再挡路。旧文件只有 `At` 和 `LRC`，照读，`Asked` 空当作只有 lrclib。
- 不使用 Spotify 内部的 color-lyrics 接口（非公开，需要内部 token）。
- 逐字高亮用源给的逐字时间（YRC，或增强 LRC 的 `<mm:ss.xx>`），没有的按整行动画（`Line.Timed`）。

## 8. 平台

| | Omarchy / Linux | macOS（Apple Silicon） |
|---|---|---|
| 界面 | MyGo 原生 UI，GTK3 + OpenGL | MyGo 原生 UI，Metal |
| 播放 | Soloist（x86_64 / aarch64，PipeWire） | Soloist（osx_arm64，macOS 13+） |
| Soloist 服务 | systemd user unit | launchd agent |
| 系统媒体控制 | MPRIS（mygo 内置；v1.1 由 agent 提供） | Now Playing（同上） |
| 特有功能 | Hyprland 快捷键说明；跟随 Omarchy 主题（v1 之后） | 刘海播放器 |
| 凭据存储 | libsecret / GNOME Keyring | 钥匙串 |
| 安装 | mygo 的 `install.sh`（用户级）、`.tar.gz` | `.dmg`（签名和公证需要 Apple 开发者账号） |

## 9. 合规与风险

- **许可证**：沿用 cloudmusic 的代码，所以整个项目是 **AGPL-3.0**，保留原作者署名。备用后端用到的 go-librespot 是 GPL-3.0，与 AGPL-3.0 兼容。
- **命名**：Spotify 的开发者条款和品牌规范不允许 App 名称里包含 "Spotify"。App 名称为 MyGoify。
- **红线**：不实现 PlayPlay，不做任何形式的音频下载或导出，不缓存明文音频。音频可视化听到的声音只在内存里算频谱，留最近不到一秒，不写盘（§4.5）。Spotify 下架过的 SpotiFLAC、EeveeSpotify、SpotifyDL 都越过了这些线。
- **外部依赖风险**：
  - Soloist 是新产品，Spotify 可能改条款或停止提供 → 播放层用接口隔离，保留 §4.4 的备用后端。
  - Web API 开发模式可能继续收紧 → 数据层集中在 `library` 一个包里，方便应对。
  - Soloist 社区反馈有强制响度均衡、偶尔卡顿 → 在 PoC 阶段实测。

## 10. 待决定的问题

1. ~~App 显示名~~：已定为 MyGoify。
2. ~~代码来源~~：已 fork 为 aronnaxlin/mygoify，保留 git 历史，`upstream` 指向原仓库。
3. ~~Soloist 下载~~：**一键从官方 CDN 下载**（2026-10-31 定）。二进制直接从 Spotify 的 `soloist-builds.spotifycdn.com` 下载到用户机器，App 不转发、不打包，理解为不属于再分发。下载前说明来源并让用户确认；同时保留"选择已下载的文件"，CDN 不可用时也能装。
4. ~~API key 怎么交给 Soloist~~：PoC P9 确认只能用 `-k` 参数，同机其他用户能在 `ps` 里看到；做法见 §4.2"进程管理"，单用户机器上接受，README 写明。
