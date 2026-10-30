# AGENTS.md

## Project

MyGoify is a third-party Spotify desktop client, forked from [mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic) (remote `upstream`). Read [docs/DESIGN.md](docs/DESIGN.md) and [ROADMAP.md](ROADMAP.md) before changing architecture. Version 1 covers music only: no podcasts. Never implement PlayPlay, audio download or export, or anything that works around Spotify's restrictions.

The app (`internal/app`) asks `library.Library` for music and plays through `playback.Player`, whose `Backend` does the playing. Both have fakes (`library.Fake`, `playback.Fake`), which the app runs on until Spotify answers (M2, M3). `go tool mygo dev` with no setup shows made-up music.

## Tests

Only write a test for behaviour that can actually break. If the code under test could change without the test noticing, or the test would fail on a harmless rewording, don't write it.

**Write tests for:**

- What input does: clicks, keys, drags, gestures and scrolling, and the state or effect they lead to (seeking when the progress bar is let go, Space after opening Now Playing, back and forward, flinging the cover, liking a song, leaving a radio).
- Logic that doesn't need a view: parsing lyrics, formatting times and counts, the player's queue, seeking, fading and cache, decoding streams and images.
- A regression you just fixed. Say in the test's comment what went wrong.
- Frame budgets, in `perf_test.go`, so idle views don't start asking for frames they don't need.

**Don't write tests that:**

- Check that a list of labels shows on screen. That repeats the view code and breaks whenever the wording changes.
- Check default settings or the order of rows in a form.
- Test MyGo itself, for example that `AnimateTo` or a spring moves from 0 to 1.
- Depend on the wall clock. `ui.NewTester` starts its clock at the current time, so anything with a phase taken from the clock passes or fails depending on when the test runs. Also, `FramePending` ignores frames waiting on `After` or `Painter.After`.

**How:**

- Build views with `ui.NewTester` and set up the app with `newTestApp` (in `internal/app/helpers_test.go`). Find elements by their accessibility label, then `Click`, `RightClick`, `Press`/`Move`/`Release` or `Key`, and check the app's state afterwards.
- The app of `newTestApp` asks a `stubLib` (in `helpers_test.go`), which answers as `library.Fake` does: set one of its functions, as `stub(a).like`, to see what the app asks or to answer otherwise. Swap out the clipboard through `copyText`, and restore it with `defer`.
- Set up what plays with the player's fixtures (`Cue`, `Hold`, `Start`) and lyrics with `a.setLyrics`.
- Look at visuals instead of asserting on them. The `NM_FRAMES` tests save PNGs into that directory and skip when it isn't set. Check pixels only for coverage, such as whether a layer still covers the window.
- Start each test with a one-sentence comment saying the behaviour it checks, in the style of the code.
- Run `go test ./internal/...` before committing, and run a new UI test with `-count=5`. Fix or delete a flaky test; don't retry it.
