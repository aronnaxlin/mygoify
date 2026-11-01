# ROADMAP

设计见 [docs/DESIGN.md](docs/DESIGN.md)。v1.0 只做音乐：播放、曲库、歌单、搜索、歌词。不做播客。

每个里程碑都要通过退出条件才能进入下一个。时间是按一个人全职的粗略估计。

---

## M0 · PoC：验证关键假设（约 1 周）

不写界面，只用命令行小程序验证。**任何一项不通过，就回到设计文档重新评估。**

| # | 验证内容 | 通过标准 |
|---|---|---|
| P1 ✅ | Omarchy 上运行 Soloist，`--ws 127.0.0.1:0`，用 Go 连上 WebSocket | 能收到 `auth_state` 和 `playback_state` |
| P2 ✅ | 首次配对：手机 App 选中 Soloist 后，重启 Soloist 会话仍在 | 重启后 `logged_in: true` |
| P3 ✅ | Web API PKCE，用自己的 client ID 登录 | 拿到 token，能调 `/me/playlists` 和 `/search` |
| P4 ✅ | Web API `PUT /me/player/play`，带 `context_uri` + `offset` + Soloist 的 `device_id` | 从歌单第 N 首开始播放，WebSocket 收到对应事件 |
| P5 ⏸ 推迟 | 不用手机完成配对：App 作为 zeroconf 发送方交给 Soloist 凭据 | 可选，失败也不阻塞，退回手机配对 |
| P6 ✅ | 用 `position_sync` 在本地插值进度，和实际声音对比 | 误差 ≤ 100 ms，歌词同步够用 |
| P7 ✅ | 资源：Soloist 播放时的 PSS 和 CPU，是否强制响度均衡、seek 延迟 | 记录数据，写进 `docs/BENCHMARK.md` |
| P8 ✅ | Apple Silicon 的 Mac 上重复 P1、P4 | 通过 |
| P9 ❌ 见 POC.md | API key 怎么交给 Soloist 才不出现在 `ps` 里 | 找到一种可行方式 |

**退出条件**：P1–P4、P6、P7 通过；P5、P8、P9 有结论。进展见 [docs/POC.md](docs/POC.md)。**M0 已完成（2026-10-30）**：P5 推迟，P9 不通过但有对策，其余通过。

---

## M1 · 骨架（约 1 周）

- [x] Fork `egoist/mygo-cloudmusic` 为 `aronnaxlin/mygoify`，保留历史；改模块名、`mygo.json`（显示名、identifier、slug、更新公钥）
- [x] 新图标；官网（`website/`）改造或删除
- [x] 删除评论、热评、播客、字幕和语音识别（`speech`）、私人 FM 和心动模式、日推、用户页、动态、搜索建议的界面和代码（DESIGN §6）
- [x] 删除 `netease`、`player`（随下面的接口替换完成）
- [x] 新建 `internal/music`（数据模型）、`internal/library`、`internal/playback`（接口 + 假实现）
- [x] 新建 `internal/auth`（PKCE，从 `cmd/poc` 挪入；接入 App 和存进钥匙串在 M2）
- [x] 界面全部改为只依赖接口，能在假数据下运行和测试
- [ ] 音频可视化（播放条音柱、刘海音柱、背景随低音起伏的雾）（macOS 已完成：process tap 听 Soloist 的声音算频谱，DESIGN §4.5；Linux 未做，打算读 PipeWire 里 Soloist 的流）
- [x] 测试规范沿用 cloudmusic 的 `AGENTS.md`（测试改用 `stubLib` 和播放器夹具）
- [x] CI：`go test ./...`，Linux 和 macOS 都要 `CGO_ENABLED=0` 交叉编译通过（`.github/workflows/ci.yml`）
- [x] 在假数据上重建截图工具：`NM_SHOTS=<目录> go run .`（`internal/app/debug_shots.go`）

**退出条件**：`go tool mygo dev` 能在假数据下打开完整外壳；测试全部通过。

---

## M2 · 登录与播放（约 2 周）

- [x] 引导页：创建应用、填 client ID → PKCE 登录 → 下载或选择 Soloist → 填 API key → 配对（#20；走完后重启 App 生效，不热切换；macOS 上新用户从零走完已实测，能播放；2026-11-01 Omarchy 上从零走完：下载的 Linux 版 Soloist 能运行，登录和 key 存进 gnome-keyring，手机配对后重启直接能播）
- [x] 凭据存进钥匙串，token 自动刷新（macOS 钥匙串已实测；2026-11-01 Omarchy 上对 gnome-keyring 实测：存取、删除、从文件搬进钥匙串都正常，`MYGOIFY_KEYCHAIN=1 go test ./internal/secret` 可重测）
- [x] 填 key 前停掉服务，在 data-dir 副本上验证（`Service.ChangeKey`，引导页的 key 步骤已接入；macOS 上 `CheckKey` 真 Soloist 实测通过；2026-11-01 Omarchy 上引导页里换 key 实测：错的 key 只碰副本，配对和钥匙串里的 key 不变；对的 key 停服务约 2 秒。不成样子的 key，Soloist 报的是 `make sure --api-key is valid`，以前没认出来，要等满 30 秒并提示"没能启动"，#26 已修）
- [ ] Soloist 服务管理：systemd user unit / launchd agent，安装、启动、停止（App 启动时安装并拉起已完成，macOS 已实测；2026-11-01 Omarchy 实测：2 秒内装好并拉起，unit 不含 key，配置不变时重开 App 不重启 Soloist，崩溃后 1 秒内重启并恢复会话，退出码 10 不重启，三种退出方式、关窗时退出、切换登录时启动都符合预期，登录时真的自启未实测（要注销）；设置页"播放"可选退出时停止 / 暂停 / 继续播放、关窗时退出、登录时启动 Soloist，均已有单元测试；2026-11-01 macOS 真机实测：三种退出方式和关窗时退出符合预期，登录时启动 Soloist 未实测（要注销或重启）；key 在钥匙串，路径和设备名在设置文件，`.env` 只在开发时补空缺）
- [ ] Soloist 和账户相关的配置全部运行时读取（DESIGN §4.2 "运行时配置"）：设置页可更换 key、可执行文件、设备名（key 和可执行文件可在引导页单步重做，从设置页"账号"进入；设备名还不能改；改完要重启 App）
- [ ] 过期处理：`soloist --version` 算出到期日，设置页显示，到期前 14 天提醒；退出码 `10` → 只走"更新 Soloist"一步（launchd 和 systemd 已设置为退出码 10 时不重启；到期日、提醒和更新流程未做）
- [ ] 实测并处理：key 失效或填错时 Soloist 的表现；换新版本后配对会话是否保留（key 填错已实测：会删掉配对会话，见 POC.md；换版本待测）
- [x] `playback` 的 Soloist 实现：WebSocket 客户端、断线重连、状态插值
- [x] 播放条：播放和暂停、上一首和下一首、seek、音量、随机、循环
- [x] 正在播放页：封面、背景、进度
- [x] 队列：从 `queue_changed` 读取，显示在封面墙上；"加入队列"
- [x] 少用 Web API 播放：从头播放专辑或歌单走 WebSocket；Soloist 的设备 ID 存盘，启动时不再查设备列表；限流时点中间某一首直接提示"限流中"，不退回本地播放，以免打乱其他设备的播放来源和队列；空闲时不发请求的测试（DESIGN §4.2）
- [x] MPRIS / Now Playing、媒体键（mygo 内置）
- [x] 关窗口缩到托盘（菜单栏或通知区域图标、托盘菜单；没有托盘的 Linux 桌面上关窗口直接退出）
- [ ] 只开一个实例：再启动时把已开的窗口叫到前面，自己退出。现在没有拦（2026-11-01 `go tool mygo dev` 一次起了两个，都连着 Soloist、都能发命令）；打包的 `.app` 双击时系统会切到已开的那个，dev 和直接运行可执行文件时没有

**退出条件**：新用户按引导页从零走完，能播放一首歌，媒体键可用。

---

## M3 · 曲库与歌单（约 2 周）

- [ ] 请求管线：去重、缓存、限流（Retry-After 加抖动、最多 4 次、`QUOTA_EXCEEDED` 单独处理）（限流已完成：要等的比调用能等的久就立刻报"限流中"，等到何时存盘、重启后照样等；磁盘缓存已完成：首页 5 分钟、歌单列表 10 分钟、专辑 7 天、歌单按 snapshot_id 复用曲目，读失败时拿旧的顶上；艺人和搜索不缓存；真实账号实测：空缓存浏览一遍 60 次调用，其中喜欢的歌曲 43 次，有缓存再来一遍 9 次；跨调用去重未做；同时最多 2 个请求、遇到 429 降到 1 个，以及修改类请求遇到 429 不重试，也未做）
- [x] 首页：最近播放、常听艺人和曲目、我的歌单
- [x] 侧边栏：我创建的歌单、收藏的歌单、喜欢的歌曲
- [x] 歌单页：自己的或协作的显示完整曲目；别人的只显示信息和播放按钮
- [x] 点击任意一首 → 从该曲开始播放所在的 context
- [x] 喜欢和取消喜欢（`/me/library`）
- [ ] 喜欢的歌曲超过 500 首时：列表分页加载；心形改为按显示的曲目查询 `/me/library/contains`，不再预先读入全部（现在存盘后只从第一页读到上次最新的那首，通常一次调用；总数对不上或满 24 小时才全部重读，最多 10000 首，四页并发；首次仍要等全部读完）
- [x] 歌单编辑（只限自己的）：新建、改名、添加、删除曲目
- [x] 专辑页、艺人页（专辑列表 + 用搜索拼出的"热门"）
- [x] 歌曲右键菜单：加入队列、加入歌单、去专辑、去艺人、复制链接
- [ ] 为你推荐（daylist、Discover Weekly、Daily Mix）：开发模式的 Web API 对 `37i9` 个性化歌单返回 404，改走 Soloist。2026-11-01 实测：按 URI `play` 能播 daylist（`37i9dQZF1EP…`，从最近播放里拿到）和 Discover Weekly（`37i9dQZEVX…`，用户贴的分享链接）；context 事件带名字（Discover Weekly 刚开播的一两个事件里为空），不带封面；`get_queue` 列出全部曲目（来源都是 `context`）。封面：oEmbed（`open.spotify.com/oembed`，免 token）给 Discover Weekly 官方封面，daylist 只给通用名和默认图，对不上当期，退回曲目封面拼图。Daily Mix 未测（缺链接）。链接当作会过期的缓存：Discover Weekly、daylist 每个账号一个、ID 应该稳定（未验证）；Daily Mix 按口味分组，可能换 ID 或消失；链接只属于当前账号。做法：从最近播放和 Soloist 的 context 事件里认出 `37i9` 歌单就记下或更新，按账号存；失效时卡片上提示重新粘贴分享链接，不报错；加 `MYGOIFY_REAL` 测试定期检查记下的链接还能不能播。动手前先改 DESIGN.md:180

**退出条件**：日常听歌不需要打开官方客户端；连续使用 1 小时不触发 `QUOTA_EXCEEDED`。

---

## M4 · 搜索（约 1 周）

- [x] 搜索曲目、专辑、艺人、歌单；每类 10 条，"显示更多"翻页
- [x] 按回车才搜，不做输入时实时搜索
- [x] 搜索历史（本地，最多 20 条，可清空）
- [x] 结果直接播放、加入队列、加入歌单

**退出条件**：常见搜索 1 秒内出结果；翻页没有重复和遗漏。（2026-11-01 真实账号实测通过：3 个关键词×4 类，第一页最慢 854 ms；翻页曾因跳过已下架的歌单而重复，已修；搜索结果不缓存；`MYGOIFY_REAL=1 go test ./internal/spotify -run Real` 可重测）

---

## M5 · 歌词（约 1 周）

- [x] LRCLIB 客户端：精确匹配 → 搜索 → 去后缀再搜 → 只按歌名搜；磁盘缓存，查不到的结果 7 天后重试
- [x] LRCLIB 不稳的兜底：预取下一首、问不到在后台再试、骨架屏最多 4 秒、设置里可换服务器地址
- [x] 正在播放页的歌词面板（沿用 cloudmusic）
- [x] 命中率：2026-11-01 抽 50 首已点赞歌曲。只有 LRCLIB 时找到 32、没有 7、网络错误 11；接上网易云和 QQ 后同一天再抽 50 首，NetEase 找到 48（其中 32 首有逐字、28 首有翻译）、QQ 47、LRCLIB 20，至少一个源找到 49/50（DESIGN §7）
- [x] 多个歌词源：`lyrics.Source` 与 `lyrics.Sources` 组合层（同时问；拿到逐字时间和翻译就返回，否则最多再等 1.5 秒），缓存移到组合层；网易云默认开，QQ 音乐默认关（设置里各一个开关）
- [x] 歌词舞台（`ui_stage`）：整行动画；LRC 带增强时间戳时逐字高亮（沿用 cloudmusic，`internal/lyrics` 解析逐字时间）
- [x] 点击歌词跳转到对应位置

**退出条件**：热门曲目的歌词命中率达到可用水平（抽样 50 首，记录命中率）；同步误差肉眼看不出。

---

## M6 · v1.0 发布（约 1 周）

- [ ] 内存门槛：Linux 上窗口打开时界面 PSS ≤ 200 MB，写进 `perf_test.go` 或基准脚本
- [ ] 空闲 CPU：窗口静止时不请求新帧（沿用 cloudmusic 的帧预算测试）（`perf_test.go` 已覆盖正在播放页和背景；其他页面、窗口隐藏时的 CPU 未测）
- [ ] 打包：Linux `.tar.gz` + `install.sh`；macOS arm64 `.dmg`（有 Developer ID 就签名和公证，升级不弹钥匙串授权；没有就 ad-hoc 签名，每个新版本弹一次，README 写明 Gatekeeper 怎么放行）
- [ ] 签名的自动更新（mygo update，配置 Ed25519 公钥）（公钥已写进 `mygo.json`，App 已接入更新器；还没发布过版本，未实测）
- [ ] README：安装、申请 Spotify 开发者应用和 Soloist key 的步骤、已知限制（§5.3）、Hyprland 快捷键示例
- [ ] CHANGELOG、Release 流程（沿用 cloudmusic 的 `release.yml`）（两者都已沿用；`release.yml` 还带着 Windows 构建，未按 v1 的平台调整，也没走过一次）

**退出条件**：在一台干净的 Omarchy 和一台干净的 Mac 上，从下载到听歌一次走通。

---

## v1.0 之后

| 版本 | 内容 |
|---|---|
| v1.1 | `mygo-spotify-agent`：后台提供 MPRIS / Now Playing；关窗口时界面进程直接退出，常驻 ≤ 80 MB |
| v1.2 | Spotify Connect 设备切换（播到其他设备、从其他设备接管） |
| v1.3 | 跟随 Omarchy 主题配色；Waybar 模块示例 |
| v1.4 | go-librespot 纯 Go 备用后端（DESIGN §4.4），支持 Intel Mac |
| 待定 | 歌词源插件化：主程序只留 LRCLIB 和插件机制（goja 跑 `.js` 插件，只给 `fetch` 和 md5、AES、base64；不能读文件、拿不到凭据；导入后默认关闭），非公开源改成插件，放在另一个仓库，不随主程序发布。在此之前非公开源先内置（2026-11-01 决定） |
| 待定 | Windows 发布；律动视图；播客（明确排在最后） |

**明确不做**：下载和导出音频、PlayPlay、绕过任何 Spotify 限制。
