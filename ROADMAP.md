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
- [ ] 新图标；官网（`website/`）改造或删除
- [x] 删除评论、热评、播客、字幕和语音识别（`speech`）、私人 FM 和心动模式、日推、用户页、动态、搜索建议的界面和代码（DESIGN §6）
- [x] 删除 `netease`、`player`（随下面的接口替换完成）
- [x] 新建 `internal/music`（数据模型）、`internal/library`、`internal/playback`（接口 + 假实现）
- [x] 新建 `internal/auth`（PKCE，从 `cmd/poc` 挪入；接入 App 和存进钥匙串在 M2）
- [x] 界面全部改为只依赖接口，能在假数据下运行和测试
- [ ] 音频可视化（播放条音柱、刘海音柱、背景随低音起伏的雾）：Soloist 不提供音频数据，`Levels()` 暂无数据，界面退回空闲动画；以后考虑按节奏模拟
- [x] 测试规范沿用 cloudmusic 的 `AGENTS.md`（测试改用 `stubLib` 和播放器夹具）
- [x] CI：`go test ./...`，Linux 和 macOS 都要 `CGO_ENABLED=0` 交叉编译通过（`.github/workflows/ci.yml`）
- [ ] 在假数据上重建截图工具（原 `debug_shots.go` 依赖网易云日推，已删除）

**退出条件**：`go tool mygo dev` 能在假数据下打开完整外壳；测试全部通过。

---

## M2 · 登录与播放（约 2 周）

- [ ] 引导页：创建应用、填 client ID → PKCE 登录 → 下载或选择 Soloist → 填 API key → 配对
- [ ] 凭据存进钥匙串，token 自动刷新
- [ ] Soloist 服务管理：systemd user unit / launchd agent，安装、启动、停止、识别过期退出码 `10` 并提示更新
- [ ] `playback` 的 Soloist 实现：WebSocket 客户端、断线重连、状态插值
- [ ] 播放条：播放和暂停、上一首和下一首、seek、音量、随机、循环
- [ ] 正在播放页：封面、背景、进度
- [ ] 队列：从 `queue_changed` 读取，显示在封面墙上；"加入队列"
- [ ] MPRIS / Now Playing、媒体键（mygo 内置）
- [ ] 关窗口缩到托盘

**退出条件**：新用户按引导页从零走完，能播放一首歌，媒体键可用。

---

## M3 · 曲库与歌单（约 2 周）

- [ ] 请求管线：去重、缓存、限流（Retry-After 加抖动、最多 4 次、`QUOTA_EXCEEDED` 单独处理）
- [ ] 首页：最近播放、常听艺人和曲目、我的歌单
- [ ] 侧边栏：我创建的歌单、收藏的歌单、喜欢的歌曲
- [ ] 歌单页：自己的或协作的显示完整曲目；别人的只显示信息和播放按钮
- [ ] 点击任意一首 → 从该曲开始播放所在的 context
- [ ] 喜欢和取消喜欢（`/me/library`）
- [ ] 歌单编辑（只限自己的）：新建、改名、添加、删除曲目
- [ ] 专辑页、艺人页（专辑列表 + 用搜索拼出的"热门"）
- [ ] 歌曲右键菜单：加入队列、加入歌单、去专辑、去艺人、复制链接

**退出条件**：日常听歌不需要打开官方客户端；连续使用 1 小时不触发 `QUOTA_EXCEEDED`。

---

## M4 · 搜索（约 1 周）

- [ ] 搜索曲目、专辑、艺人、歌单；每类 10 条，"显示更多"翻页
- [ ] 按回车才搜，不做输入时实时搜索
- [ ] 搜索历史（本地）
- [ ] 结果直接播放、加入队列、加入歌单

**退出条件**：常见搜索 1 秒内出结果；翻页没有重复和遗漏。

---

## M5 · 歌词（约 1 周）

- [ ] LRCLIB 客户端：精确匹配 → 搜索兜底；磁盘缓存，查不到的结果 7 天后重试
- [ ] 正在播放页的歌词面板（沿用 cloudmusic）
- [ ] 歌词舞台（`ui_stage`）：整行动画；LRC 带增强时间戳时逐字高亮
- [ ] 点击歌词跳转到对应位置

**退出条件**：热门曲目的歌词命中率达到可用水平（抽样 50 首，记录命中率）；同步误差肉眼看不出。

---

## M6 · v1.0 发布（约 1 周）

- [ ] 内存门槛：Linux 上窗口打开时界面 PSS ≤ 200 MB，写进 `perf_test.go` 或基准脚本
- [ ] 空闲 CPU：窗口静止时不请求新帧（沿用 cloudmusic 的帧预算测试）
- [ ] 打包：Linux `.tar.gz` + `install.sh`；macOS arm64 `.dmg`（签名和公证）
- [ ] 签名的自动更新（mygo update，配置 Ed25519 公钥）
- [ ] README：安装、申请 Spotify 开发者应用和 Soloist key 的步骤、已知限制（§5.3）、Hyprland 快捷键示例
- [ ] CHANGELOG、Release 流程（沿用 cloudmusic 的 `release.yml`）

**退出条件**：在一台干净的 Omarchy 和一台干净的 Mac 上，从下载到听歌一次走通。

---

## v1.0 之后

| 版本 | 内容 |
|---|---|
| v1.1 | `mygo-spotify-agent`：后台提供 MPRIS / Now Playing；关窗口时界面进程直接退出，常驻 ≤ 80 MB |
| v1.2 | Spotify Connect 设备切换（播到其他设备、从其他设备接管） |
| v1.3 | 跟随 Omarchy 主题配色；Waybar 模块示例 |
| v1.4 | go-librespot 纯 Go 备用后端（DESIGN §4.4），支持 Intel Mac |
| 待定 | Windows 发布；律动视图；播客（明确排在最后） |

**明确不做**：下载和导出音频、PlayPlay、绕过任何 Spotify 限制。
