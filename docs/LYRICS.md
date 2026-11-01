# 歌词接入说明

接手加歌词源的人先读这一篇，再读 [DESIGN.md](DESIGN.md) §7、§9 和 [AGENTS.md](../AGENTS.md)。本文写于 2026-11-01，对应分支 `feat/lyrics`（PR #24）。PR 合并前，请从 `feat/lyrics` 开分支；合并后从 `main` 开。

## 1. 现状

> 2026-11-01 起有三个源，`lyrics.Sources` 组合，见 DESIGN §7。本节保留接入前的基线。

- 第一版只有 [LRCLIB](https://lrclib.net)：开放、免 key，只取逐行同步的 `syncedLyrics`。
- 已点赞歌曲抽 50 首（中日英混合，2026-11-01）：找到 32 首，LRCLIB 没有 7 首，网络错误 11 首。
  - 网络错误是 503 或连接断开，每首约 15 秒后放弃。
  - 耗时中位数约 2 秒，p90 约 15 秒。
- 没有逐字歌词：LRCLIB 只给逐行时间。界面上的逐字动画用的是平摊出来的假时间，见 §6.3。
- 兜底已经做了：预取下一首、后台重试、骨架屏最多 4 秒、可换 LRCLIB 地址（§5）。
- 加别的源，就是为了补上这两块：LRCLIB 没有的歌，以及逐字时间和翻译。
- 现在：网易云（默认开，逐字 YRC + 翻译）和 QQ 音乐（默认关，逐行 + 翻译）已接上；同一天再抽 50 首，至少一个源找到 49 首，数字见 DESIGN §7。

## 2. 红线和已定的事

**不能做（AGENTS.md、DESIGN §9）**
- 不绕过 Spotify 的任何限制，不下载或导出音频。
- 不用 Spotify 内部的 color-lyrics 接口，也不用 `sp_dc` Cookie。DESIGN §7 写明："非公开，需要内部 token"。
- 不用 cgo。CI 用 `CGO_ENABLED=0` 构建。
- 日志和输出里不能出现任何 token、cookie 或 key。

**用户还没拍板，动手前先问**
- 非公开来源要不要接、接哪些：网易云、QQ 音乐、酷狗、Musixmatch 的非官方接口、Apple Music 的 Media User Token。
- 之前给用户的建议有三条：
  - 真要接，做成设置里**默认关闭**、由用户自己打开的选项，并在 DESIGN §7 写清楚风险。
  - **不做 QRC 解密**：QQ 音乐的逐字歌词是加密的。
  - **不用 `sp_dc`**。
- 2026-11-01 用户定了：**网易云默认开，QQ 音乐默认关**，先做这两个；酷狗、Musixmatch、Apple Music 不做（KRC 要解码、Musixmatch 要非官方 token、Apple 要用户自己的订阅 token）。

**候选来源（参考 [Lyricify Lyrics Helper](https://github.com/WXRIW/Lyricify-Lyrics-Helper)，Apache-2.0）**

| 来源 | 凭据 | 能拿到什么 | 备注 |
|---|---|---|---|
| LRCLIB | 无 | 逐行、纯文本 | 已接。官方开放 |
| 网易云 | 无 | 逐行 LRC、逐字 YRC、翻译 | 已接，默认开。非公开接口；`Build` 现成支持 YRC 和翻译（从 cloudmusic 继承） |
| QQ 音乐 | 无 | 逐行 LRC、翻译 | 已接，默认关。逐字是 QRC（加密），不做 |
| 酷狗 | 无 | 逐字 KRC | 不做：KRC 也要解码，中文库和网易云重叠 |
| Musixmatch | 非官方 user token | 逐行、逐字、翻译 | 不做：官方 API 免费档只给预览；完整同步歌词要签商用授权，起价据二手资料约 2.5 万美元 |
| Apple Music | 用户的 Media User Token | TTML 逐字 | 不做：要用户自己的订阅 token |
| Spotify | `sp_dc` | 逐字、逐行 | **不做** |

上游 egoist/mygo-cloudmusic 有网易云取歌词的代码，在提交 eb97f6d 里随 `internal/netease` 一起删掉了。只剩解析部分：`parseYRC`、`Build` 的翻译合并。
- 想参考可以看 `git show eb97f6d^:internal/netease/netease.go`：`Lyrics(id int64)` 在约 459 行，返回 lrc、翻译和 yrc，加密请求的 `eapiPost` 也在这个文件里。
- 它按网易云曲目 ID 取，不能直接用：MyGoify 只有 Spotify 的曲目信息，要先搜索匹配（§5、§6.1）。

## 3. 代码地图

| 位置 | 作用 |
|---|---|
| `internal/lyrics/lyrics.go` | 数据模型和解析：`Line`、`Word`、`parseLRC`（含增强 LRC 的 `<mm:ss.xx>`）、`parseYRC`、`Build(lrc, translation, yrc)`、`SpreadWords`、`Index`、`Quiets`、`IsCredit` |
| `internal/lyrics/sources.go` | `Source` 接口、`Found`、组合层 `Sources`（挑逐字/逐行/翻译）、匹配（`pick`、`nearestOf`）、磁盘缓存（`load`、`store`） |
| `internal/lyrics/lrclib.go` | LRCLIB 客户端：`lookup`、`best`、`bareName`、重试（`get`） |
| `internal/lyrics/netease.go` | 网易云：搜索、eapi 签名（`eapiSign`）、一次取 LRC + 翻译 + YRC |
| `internal/lyrics/qq.go` | QQ 音乐：mobile 搜索、`fcg_query_lyric_new.fcg`、base64 + HTML 实体 |
| `internal/lyrics/*_test.go` | 用 httptest 假服务器测匹配、缓存、错误；`netease_test.go` 里有一份 eapi 签名的已知答案 |
| `internal/app/lyrics.go` | App 这边的接入：`fetchLyrics`（包变量）、`lyricsSources`、`useLyricSources`、`playbackChanged`、`askLyrics`、`currentLyrics`、`setLyrics` |
| `internal/app/lyrics_test.go` | `newLyricsRig`：手动驱动请求、post 和定时器，测兜底 |
| `internal/app/app.go:217` | 连上 Spotify 后调用 `useLyricSources()`；假曲库（`go tool mygo dev` 没配置时）不接，`fetchLyrics` 一直返回空 |
| `internal/app/app.go:221` | `a.pl.Changed = a.playbackChanged` |
| `internal/app/settings.go` | `settings.LyricsServer` |
| `internal/app/ui_settings.go:~141` | 设置页"歌词"分组 |
| `internal/app/ui_nowplaying.go:329` | 歌词面板 `lyricsPane`，逐字填色在约 403 行 |
| `internal/app/ui_stage.go`、`ui_stage_styles.go` | 律动舞台：经典 `stageLine`、云阶 `partitaLine`、倾斜 `tiltLine` |
| `internal/app/ui_wall.go:158` | 选歌墙也读 `currentLyrics()` |

## 4. 数据流

1. 换歌时 `Player` 调 `a.playbackChanged()`，在主线程上执行。
2. `playbackChanged` 依次：
   - 把 `a.lyr` 重置为 `lyricsState{track: id}`，此时 `loaded=false`，界面显示骨架屏；
   - 调 `askLyrics(当前曲目, 0)`；
   - 4 秒后（`lyricsLater(lyricsWait, …)`）若还是这首，就设 `loaded=true`；
   - 按 `PlayOrder()` 找到下一首，也调 `askLyrics`，结果只为写进磁盘缓存。
3. `askLyrics`：
   - 用 `a.lyrAsking` 去重，同一首同时只问一次；
   - 在主线程读出 `a.settings` 里的 `lyricsSources`（服务器地址和两个开关），然后开 goroutine 调 `fetchLyrics(track, src)`；
   - 结果通过 `a.post` 回到主线程：不是当前曲目就丢掉；是的话设 `lines` 和 `loaded=true`；出错就按 `lyricsRetries` 排一次重试（30 秒、1 分钟、2 分钟）。
4. `fetchLyrics` 由 `useLyricSources` 设置。它按设置拼出 `lyrics.Sources`（网易云在前，LRCLIB 其次，QQ 打开时加在最后），共用一个 20 秒超时的 `http.Client`，在 1 分钟的 ctx 下调 `Find`。
5. `Sources.Find` 先查磁盘缓存；没有再同时问所有源，挑逐字、逐行和翻译，存缓存，最后用 `Build` 转成 `[]Line`。拿到逐字时间和翻译就立刻返回，取消其余请求；只有逐字或只有逐行时再等 1.5 秒看有没有更好的，到点就返回，不必等 LRCLIB 超时——网易云 0.3 秒找到的歌词不该晚一分钟才显示。

**返回值约定（加源时要保持）**
- `lines, nil`：有歌词。
- `nil, nil`：确定没有。界面显示"暂无歌词"，不重试。
- `nil, err`：**没问成**（网络、503）。不缓存，App 会后台重试，下次播到这首也会再问。
- 不要把"没问成"当成"没有"：那样会被缓存 7 天。

**线程**
- `fetchLyrics` 在 goroutine 里跑，不能读写 `a` 的任何字段。
- 要用的设置必须像 `server` 那样，在主线程先取出来再传进去。

## 5. LRCLIB 细节和实测到的坑

- 查找顺序（`lookup`）：
  1. `/api/get` 精确匹配，参数是 `track_name`、`artist_name`、`album_name`，以及整数秒的 `duration`。
  2. `/api/search` 按歌名和第一个艺人搜，取时长差 3 秒内最近的；一样近时取同专辑的。
  3. 歌名去掉 ` - Remastered 2011`、` (feat. …)` 这类后缀（`bareName`），再搜一次。
  4. 只按歌名搜，时长差 1 秒内才算。
- **503**：`/api/get` 遇到库里没有的歌，会去别处找，常常一秒后回 503 `ServerOverloaded`。所以 get 只试一次，503 直接转去搜索。搜索遇到 503 或 429 才退避重试（1、2、3、4 秒，最多 5 次）。
- `/api/get-cached` 没用：已知的歌也回 404。
- LRCLIB 要求带 User-Agent，现在发的是 `MyGoify (https://github.com/aronnaxlin/mygoify)`。
- 艺人名写法不同：Spotify 写 `Rene Liu`、`Hebe Tien`，LRCLIB 写 `刘若英`、`田馥甄`，所以有第 4 步。
- 数据质量：有的条目是繁体中文；有的在 00:00 放一行标题或制作人员。`IsCredit` 只认"作词："这类格式。
- **缓存**（2026-11-01 起在组合层 `lyrics.Sources`）：
  - 位置：`os.UserCacheDir()/mygoify/lyrics/<Spotify 曲目 ID>.json`，内容是 `{"At", "LRC", "Translation", "YRC", "Source", "Asked"}`。
  - `LRC` 或 `YRC` 不为空就一直留着；都为空表示"没有"，7 天后再问，且 `Asked`（写的时候启用了哪些源）和当前开关不一致时立即重问；错误不缓存。
  - 旧文件只有 `At` 和 `LRC`，照读；`Asked` 空当作只有 lrclib。
  - 写入时先写 `.tmp` 再改名，文件权限 0600。
  - 曲目 ID 含 `/`、`\` 或 `.` 时不缓存（防路径穿越）。
- 时长：`music.Track.Duration` 是毫秒，`Query.Duration` 是 `time.Duration`。
- 曲目信息里没有 ISRC：`music.Track` 只有 ID、Name、Artists、Album、Duration。有的源能按 ISRC 精确查，要用就得在 `internal/library` 里把 Web API 的 `external_ids.isrc` 带出来。注意 library 是 Spotify 数据的唯一入口（DESIGN §3）。

## 6. 加新源要改的地方

### 6.1 建议的结构（2026-11-01 已按此实现）

现在只有一个具体类型 `LRCLIB`，缓存也写在它里面。加源前先抽出来：

```go
// internal/lyrics
type Source interface {
    Name() string                                   // 写进缓存和日志
    Find(ctx context.Context, q Query) (Found, error) // 约定同 §4：没有就返回零值和 nil
}

type Found struct {
    LRC, Translation, YRC string // 或者直接 []Line，看格式
}
```

- **组合**：按设置里的顺序依次问各个源。
  - 有逐字时间的优先；只有逐行时，翻译可以取自另一个源，`Build` 已按时间就近合并翻译。
  - 任何一个源出错（不是"没有"）时，整体要算"没问成"，不能缓存成"没有"，否则那个源恢复后要等 7 天。
- **缓存从 `LRCLIB` 移到组合那一层**，按曲目 ID 存。
  - 记录来自哪个源和格式。
  - 现在的格式只有 `At`、`LRC`，要加字段；旧文件要能读：缺字段就当 LRCLIB。
  - 用户改了源的开关之后，旧的"没有"记录应该失效，比如在记录里写下当时启用了哪些源。
- **`app` 这边**：
  - `fetchLyrics(track, server)` 可以改成传一个从设置里取出的源配置（主线程取值）。
  - `useLRCLIB` 改名为类似 `useLyricSources` 的名字。
  - `askLyrics` 的去重、重试、预取不用动。
- **设置**：
  - 每个非公开源一个开关，默认关闭，说明写清来源和风险。
  - 加在 `ui_settings.go` 的"歌词"分组里，用 `row(标题, 说明, 控件)` 和 `ui.Switch(...).Changed()` 再 `a.changed()`，照着已有的行写。
- **匹配**：大多数源也只能搜索，可以复用 `best` 的思路：时长 ±3 秒，按专辑决胜负，按歌名单独搜时 ±1 秒。

### 6.2 格式

- `Build(lrc, translation, yrc)`：有 YRC 就用逐字，没有就用 LRC 加 `SpreadWords`。`Line.Timed` 标记是不是真的逐字时间。
- YRC 格式：`[开始,时长](开始,时长,0)字(开始,时长,0)字…`，单位是毫秒。
- 新的逐字格式（TTML、KRC、Musixmatch richsync、增强 LRC 的 `<mm:ss.xx>`）要各写一个 parser，产出 `Timed: true` 的 `Line`，在 `lyrics_test.go` 里测解析。
- 现在的 `parseLRC` **不认**增强 LRC 的 `<mm:ss.xx>`：这种标签会原样留在文字里。LRCLIB 上少数条目带这种标签，可以顺便支持。

### 6.3 已知问题：逐行歌词和律动不搭（用户已知，暂不处理）

- 没有逐字时间的行，`SpreadWords` 会把一行（最长 6 秒）的 85% 时间按字数平均分给每个字。
- 律动的三种样式和歌词面板都按这个假时间一个字一个字地弹入或填色，和真实演唱对不上。
- DESIGN §7 写的是"逐字高亮只在 LRC 带增强时间戳时启用；否则舞台按整行动画"，现在的实现没做到这一条。
- 用户这次没让改。接上有逐字时间的源之后，这些歌的问题自然消失。别顺手改动画，要改先问用户。

## 7. 测试

先读 AGENTS.md 的 Tests 一节。要点：
- 只测会坏的行为；每个测试开头一句话写它测什么。
- 新的 UI 测试用 `-count=5` 跑。
- 不依赖墙钟。

- **源的测试**：照 `lrclib_test.go` 的 `lrclib(t, answer)`。
  - 用 httptest 起假服务器，缓存放在 `t.TempDir()`，时钟用 `now` 字段注入，重试间隔用 `pause` 字段改成 1ms。
  - handler 里共享的计数器用 `atomic`，测试要能过 `-race`。
  - 不要在测试里访问真实网络。
- **App 层**：`newLyricsRig(t)`（`internal/app/lyrics_test.go`）。
  - 替换 `fetchLyrics`：每次请求通过 `r.asks` 交给测试，由测试决定怎么回（`r.answer`）。
  - 替换 `lyricsLater`：定时任务收进 `r.later`，由测试手动执行。
  - `a.inline` 把 post 收进 `r.posted`，用 `r.run()` 执行。
  - 加源后，组合逻辑尽量在 `internal/lyrics` 里测；App 层的测试只在改了去重、重试这类行为时才加。
- 写完测试，临时改坏被测的逻辑，确认测试会失败，再改回来。
- 提交前跑 CI 的四项，全部通过才提交：

  ```sh
  gofmt -l .
  go vet ./...
  go test ./internal/...
  CGO_ENABLED=0 go build -o /dev/null .
  ```

## 8. 实测命中率

- 用户的已点赞歌曲缓存在 `~/Library/Caches/mygoify/spotify/<用户 ID>/liked.json`（Linux 上是 `~/.cache/mygoify/spotify/<用户 ID>/liked.json`），`V.Tracks` 里约 2000 首。这是用户的曲目信息，不是密钥。
- 之前的做法：
  1. 从里面随机抽 50 首，把名字、艺人、专辑、时长写进 scratchpad。
  2. 写一个临时 `_test.go`，用环境变量控制开关，对每首调 `Find` 并记录结果和耗时。
  3. 跑完就删掉这个文件，不要提交。
- 每个源分别记录命中数、"没有"数、错误数和耗时，写进 DESIGN §7 和 ROADMAP M5。
- 中文、日文歌是重点：LRCLIB 在这块弱，用户常听。

## 9. 环境注意

- 启动方式：
  - 连上 Spotify 的开发模式：`MYGOIFY_DEBUG=1 go tool mygo dev`。
  - 没配置 Spotify 时只有假音乐，没有歌词。
- 这台机器上的 Bash 输出会被 rtk 过滤。看完整的测试输出，用 `command go test ... > 文件`，再读文件。
- `rg` 不带路径会去读 stdin，命令会一直卡住，一定要写路径。
- 不要用 `ps` 或 `pgrep -lf` 看 Soloist 进程：参数里有 API key。
- 提交、推送、开 PR 都要用户同意；`gh pr merge` 由用户自己执行。不要提交 `.claude/`。
- 回复用户用中文。
