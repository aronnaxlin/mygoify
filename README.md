English | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md)

# MyGoify

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Built with MyGo](https://img.shields.io/badge/Built%20with-MyGo-00ADD8.svg)](https://github.com/egoist/mygo)
[![Forked from egoist/mygo-cloudmusic](https://img.shields.io/badge/Forked%20from-egoist%2Fmygo--cloudmusic-lightgrey.svg)](https://github.com/egoist/mygo-cloudmusic)
[![CI](https://github.com/aronnaxlin/mygoify/actions/workflows/ci.yml/badge.svg)](https://github.com/aronnaxlin/mygoify/actions/workflows/ci.yml)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20(Apple%20Silicon)-brightgreen.svg)](#installation)
[![Playback](https://img.shields.io/badge/Playback-Spotify%20Soloist-1DB954.svg)](https://developer.spotify.com/documentation/soloist)

MyGoify is a native, lightweight third-party Spotify desktop client built with Go and [MyGo](https://github.com/egoist/mygo), focused exclusively on music playback.

The application architecture separates the user interface from the audio engine: playback is handled by Spotify's official headless player, **Soloist**, which runs as a lightweight user service. This architecture keeps memory usage significantly lower than the official client — roughly 220 MB PSS for the UI process (measured ~217 MB with the reactive smoke visual effect enabled) and ~70 MB for the background Soloist daemon (measured on Linux with the window open, compared to ~900 MB for the official desktop client). Closing the window keeps the music playing smoothly in the background.

This project was forked from [egoist/mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic). We extend our sincere gratitude to egoist for the original UI design and architecture.

> **Disclaimer**: MyGoify is an independent open-source project and is not affiliated with, endorsed by, or associated with Spotify AB. Spotify is a registered trademark of Spotify AB.

<!-- Screenshot placeholder
![Main Window (Dark)](.github/assets/main-dark.png)
![Now Playing (Light)](.github/assets/now-playing-light.png)
-->

---

## Features

- **Music Library**: Liked songs, custom and followed playlists, recently played, top artists, and top tracks.
- **Playlists**: Browse playlists, create new playlists, rename, and add or remove tracks (for user-owned playlists).
- **Search**: Search tracks, albums, artists, and playlists with local search history.
- **Lyrics**: Real-time synced lyrics with word-by-word highlights and translations when available. Powered by [LRCLIB](https://lrclib.net/) alongside optional third-party lyric sources that can be toggled in settings.
- **Playback Controls**: Full control over play/pause, next/previous, seeking, volume, shuffle, and repeat modes.
- **Queue**: Synchronized queue viewer directly reflected from Soloist, including album cover wall and "Add to Queue".
- **Desktop Integration**: Global media keys, MPRIS on Linux, Now Playing on macOS, and system tray integration (minimize to tray with continuous playback).
- **Audio Visualizer**: Real-time animated audio visualizer (player bar spectrum, macOS notch indicator, and reactive smoke background). The spectrum is computed in memory strictly from Soloist's own audio stream (via PipeWire on Linux capturing only Soloist's stream, and via Core Audio process tap on macOS targeting only the Soloist process), without capturing audio from other applications, and without recording or exporting audio files.
- **Multi-language Interface**: Multi-language support (coming soon).

---

## Prerequisites

1. **Spotify Premium**: Required for Spotify Connect and Soloist playback.
2. **Spotify Developer Application**: A registered app in the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard) to obtain a Client ID. Configure the Redirect URI as:

   ```text
   http://127.0.0.1:45231/callback
   ```

3. **Soloist API Key**: Obtained from the Spotify Developer Dashboard as described in the [Soloist documentation](https://developer.spotify.com/documentation/soloist).

MyGoify features an onboarding wizard on first launch that guides you through every step: creating your app, browser login, downloading Soloist, entering your API key, and initial device pairing.

---

## Installation

### Pre-built Releases

Pre-built packages will be available starting with the first formal release:

- **Linux**: Distributed as a `.tar.gz` archive with an `install.sh` script, installing to `~/.local` for the current user.
- **macOS (Apple Silicon)**: Distributed as a `.dmg` disk image. The app is ad-hoc signed and not notarized, as the project has no Apple Developer ID, so macOS asks you to confirm it:
  1. Drag MyGoify to Applications and open it. macOS says it "could not verify MyGoify is free of malware"; click **Done**.
  2. Open **System Settings → Privacy & Security**, scroll down and click **Open Anyway** next to MyGoify, then confirm with your password or Touch ID.
  3. On first launch, allow MyGoify to use its keychain items (enter your password and choose **Always Allow**), and allow audio recording when a song first plays: the visualizer listens to Soloist only, in memory.

  Because each version has a new ad-hoc signature, macOS asks these three again after you install a new version.

### Building from Source

You can build MyGoify directly from source using Go:

- Requires **Go 1.27+**.

```sh
# Clone the repository
git clone https://github.com/aronnaxlin/mygoify.git
cd mygoify

# Run in development mode
go tool mygo dev

# Run internal tests
go test ./internal/...

# Build production binaries
go tool mygo build
```

---

## Audio Quality & Lossless

Spotify manages streaming audio quality settings on a per-device basis.

To enable lossless audio (up to ~1.3 Mbps):

1. Start playback in MyGoify.
2. Open the official Spotify app on your phone or computer.
3. Tap the **Devices** icon (Spotify Connect), select **MyGoify**, and change the streaming quality setting to **Lossless**.
4. This configuration only needs to be completed once. Note that the Spotify web player does not support changing device quality.

By default, playback streams at approximately 160 kbps (High).

---

## Known Limitations

- **Developer Mode Web API**: Due to Spotify's Web API developer mode restrictions, search returns up to 10 results per category per page; playlists created by other users (including Spotify's algorithmic playlists such as Daily Mix) do not expose individual track lists via the API (they can still be played as a whole context); and artist top tracks are synthesised via search queries.
- **Soloist Process Parameter**: Soloist currently only accepts its API key through the command-line argument (`-k`). As a result, the key may be visible to other local users on the same machine via `ps` or `/proc`. On multi-user Linux systems, you can restrict process parameter visibility by remounting `/proc` with `hidepid=2`.
- **Music Only**: Podcasts and audiobooks are not supported.

---

## Roadmap

- **v1.0.0 (Initial Release)**: Finalize memory and idle CPU benchmarks, configure automated release packaging, and polish auto-updates.
- **v1.1.0+**:
  - Standalone background agent (`mygo-spotify-agent`) providing MPRIS / Now Playing controls when the UI is closed (<80 MB combined memory usage).
  - Recommended playlists support (daylist, Discover Weekly).
  - Spotify Connect device switching.
  - Omarchy theme synchronization and Waybar integration.
  - Pure-Go fallback backend (`go-librespot`) for broader platform compatibility.

---

## Acknowledgements

- [egoist/mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic) — Original UI codebase, interaction design, and inspiration.
- [MyGo](https://github.com/egoist/mygo) — Native desktop application framework and UI engine for Go.
- [Spotify Soloist](https://developer.spotify.com/documentation/soloist) — Official headless playback engine.
- [LRCLIB](https://lrclib.net/) — Open-source synchronized lyrics provider.
- [Lyricify Lyrics Helper](https://github.com/WXRIW/Lyricify-Lyrics-Helper) — Reference implementation for interface signatures (Apache-2.0).

---

## License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE).
