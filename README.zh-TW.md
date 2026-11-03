[English](README.md) | [简体中文](README.zh-CN.md) | 繁體中文 | [日本語](README.ja.md)

# MyGoify

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Built with MyGo](https://img.shields.io/badge/Built%20with-MyGo-00ADD8.svg)](https://github.com/egoist/mygo)
[![Forked from egoist/mygo-cloudmusic](https://img.shields.io/badge/Forked%20from-egoist%2Fmygo--cloudmusic-lightgrey.svg)](https://github.com/egoist/mygo-cloudmusic)
[![CI](https://github.com/aronnaxlin/mygoify/actions/workflows/ci.yml/badge.svg)](https://github.com/aronnaxlin/mygoify/actions/workflows/ci.yml)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20(Apple%20Silicon)-brightgreen.svg)](#安裝方式)
[![Playback](https://img.shields.io/badge/Playback-Spotify%20Soloist-1DB954.svg)](https://developer.spotify.com/documentation/soloist)

MyGoify 是一套以 Go 與 [MyGo](https://github.com/egoist/mygo) 開發的原生輕量第三方 Spotify 桌面客戶端，專注於純粹的音樂聆聽體驗。

本應用程式將介面呈現與播放引擎徹底分離：音訊播放交由 Spotify 官方無周邊播放器 **Soloist** 負責，並作為獨立的使用者層級背景服務運作。這種架構大幅降低了記憶體消耗——在 Linux 上開啟視窗時，介面程序實測約 217 MB（開啟煙霧特效時），背景常駐的 Soloist 服務約 70 MB（相較之下，官方桌面客戶端約 900 MB）。關閉介面視窗後，音樂仍會在背景持續順暢播放。

本專案 Fork 自 [egoist/mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic)。由衷感謝 egoist 所打造的優異 UI 設計與架構基礎。

> **免責聲明**：本專案為獨立開源專案，與 Spotify AB 無任何官方關聯。Spotify 為 Spotify AB 之註冊商標。

<!-- 螢幕截圖占位
![主介面（深色）](.github/assets/main-dark.png)
![現正播放（淺色）](.github/assets/now-playing-light.png)
-->

---

## 功能特色

- **音樂庫**：已按讚歌曲、自訂與收藏的播放清單、最近播放、常聽演出者與熱門曲目。
- **播放清單管理**：瀏覽播放清單，支援建立新清單、重新命名、新增或移除曲目（僅限個人建立之清單）。
- **音樂搜尋**：搜尋單曲、專輯、演出者與播放清單，支援本機搜尋歷史紀錄。
- **動態歌詞**：支援逐行同步歌詞、逐字高亮時間軸與歌詞翻譯。歌詞由 [LRCLIB](https://lrclib.net/) 提供，並支援可在設定中隨時啟閉的選用第三方歌詞來源。
- **播放控制**：播放/暫停、上一首/下一首、播放進度跳轉（Seek）、音量調整、隨機播放與循環播放模式。
- **待播清單（佇列）**：即時反映 Soloist 播放狀態的佇列檢視，提供專輯封面牆與「加入佇列」功能。
- **桌面環境整合**：全域媒體快捷鍵、Linux 平台 MPRIS 媒體控制協定、macOS Now Playing 支援，以及系統匣圖示（可最小化至系統匣並在背景持續播放）。
- **音訊視覺化效果**：播放列動態頻譜、macOS 瀏海音柱指示與隨低音起伏的背景霧氣動效；頻譜直接從 Soloist 自身的音訊流於記憶體中即時運算（Linux 上透過 PipeWire 僅擷取 Soloist 的音訊串流，macOS 上使用 process tap 僅接聽 Soloist 程序），絕不會擷取其他應用程式的聲音，也絕不錄製、儲存或匯出音訊檔案。
- **多語言介面**：多語言介面（即將支援）。

---

## 使用需求

1. **Spotify Premium 帳號**：使用 Spotify Connect 與 Soloist 播放時所必需。
2. **Spotify 開發者應用程式**：請至 [Spotify 開發者後台](https://developer.spotify.com/dashboard) 建立應用程式以取得 Client ID，並將 Redirect URI（重新導向回呼地址）設定為：

   ```text
   http://127.0.0.1:45231/callback
   ```

3. **Soloist API Key**：於 Spotify 開發者後台依據 [Soloist 官方文件](https://developer.spotify.com/documentation/soloist) 之說明取得。

MyGoify 首次啟動時會顯示設定精靈，引導你依序完成：建立應用程式、瀏覽器登入、下載 Soloist、填入 API key 以及完成首次區域網路裝置配對。

---

## 安裝方式

### 預編譯版本

預編譯安裝檔將於第一個正式版本發布後提供：

- **Linux**：提供 `.tar.gz` 壓縮封裝檔，內含 `install.sh` 安裝指令碼，執行即可安裝至目前使用者的 `~/.local` 目錄下並支援自動更新。
- **macOS (Apple Silicon)**：提供 `.dmg` 磁碟映像檔。本專案沒有 Apple Developer ID，App 採用 ad-hoc 簽署、未經公證，因此 macOS 會要求你確認：
  1. 將 MyGoify 拖進「應用程式」後開啟。系統提示「無法驗證 MyGoify 不包含惡意軟體」，點選**完成**。
  2. 開啟**系統設定 → 隱私權與安全性**，捲到下方，在 MyGoify 旁點選**強制打開**，再以密碼或 Touch ID 確認。
  3. 首次啟動時，允許 MyGoify 使用鑰匙圈中的項目（輸入密碼並選擇**永遠允許**）；首次播放時，允許錄音：視覺化效果只在記憶體中分析 Soloist 的聲音。

  每個版本的 ad-hoc 簽署都不同，因此手動安裝新版本後，這三個步驟需要再確認一次。透過 App 內更新升級時不會再經過 Gatekeeper，但鑰匙圈仍需再輸入一次密碼。

### 從原始碼編譯

你也可以直接使用 Go 工具鏈自行編譯執行：

- 環境需求：**Go 1.27+**。

```sh
# 複製儲存庫
git clone https://github.com/aronnaxlin/mygoify.git
cd mygoify

# 開發模式執行
go tool mygo dev

# 執行單元測試
go test ./internal/...

# 編譯正式發行版本
go tool mygo build
```

---

## 音質與無損設定

Spotify 針對不同裝置個別記錄串流音質偏好。

若需開啟無損音質（最高約 1.3 Mbps）：

1. 於 MyGoify 中開始播放任意曲目。
2. 在手機或電腦上開啟官方 Spotify 應用程式。
3. 點選**裝置**圖示（Spotify Connect），選擇 **MyGoify**，並將串流音質切換為**無損（Lossless）**。
4. 此設定僅需操作一次，後續於 MyGoify 播放時將自動維持無損音質（注意：Spotify 網頁版不支援變更裝置串流音質）。

預設串流音質約為 160 kbps（High）。

---

## 已知限制

- **開發模式 Web API 額度限制**：受 Spotify 開發模式 API 規範限制，搜尋結果各分類每頁最多回傳 10 筆項目（可透過換頁載入後續結果）；他人建立之播放清單（包含 Spotify 自動產生的 Daily Mix 等個人化清單）無法透過 Web API 取得曲目清單，但可直接以整個播放清單情境進行播放；演出者頁面的「熱門單曲」則是以搜尋介面聚合而成。
- **Soloist 命令列參數可見性**：Soloist 目前僅支援透過命令列引數（`-k`）接收 API key。因此在同一台機器上，其他系統使用者可透過 `ps` 或檢視 `/proc` 查閱該引數。在多使用者的 Linux 環境中，可透過以 `hidepid=2` 掛載 `/proc` 來隱藏程序引數。
- **純音樂播放**：不支援播客（Podcast）與有聲書。

---

## 開發藍圖

- **v1.0.0（首發版本）**：記憶體用量與閒置 CPU 負擔基準測試、自動化發布打包流程以及更新機制驗證。
- **v1.1.0+**：
  - 極輕量常駐代理程式（`mygo-spotify-agent`）：關閉視窗後主程式完全結束，由 agent 提供 MPRIS / Now Playing 控制，常駐記憶體總量 ≤ 80 MB。
  - 個人化推薦清單支援（daylist、Discover Weekly 等）。
  - Spotify Connect 播放裝置切換功能。
  - Omarchy 主題外觀同步與 Waybar 狀態列模組範例。
  - 純 Go 實作之後備播放核心（`go-librespot`），提供 Intel Mac 等更廣泛的硬體支援。

---

## 謝詞

- [egoist/mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic) — 原版 UI 程式碼、互動設計與靈感來源。
- [MyGo](https://github.com/egoist/mygo) — Go 原生桌面應用程式框架與介面引擎。
- [Spotify Soloist](https://developer.spotify.com/documentation/soloist) — 官方無周邊播放後端引擎。
- [LRCLIB](https://lrclib.net/) — 開放動態歌詞服務平台。
- [Lyricify Lyrics Helper](https://github.com/WXRIW/Lyricify-Lyrics-Helper) — 介面簽章實作之參考依據（遵循 Apache-2.0 條款）。

---

## 授權條款

本專案採用 [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE) 授權條款釋出。
