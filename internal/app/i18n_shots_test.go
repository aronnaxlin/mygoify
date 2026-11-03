package app

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// With NM_FRAMES set, saves the main view at narrowest window (680×580),
// settings view (560×720), onboarding and now playing in en, ja, zh-Hant
// to visually verify that longer strings do not overflow or get clipped.
func TestI18nVisualLayouts(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("NM_FRAMES not set")
	}
	_ = os.MkdirAll(dir, 0o755)

	savePNG := func(name string, img *image.RGBA) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}

	langs := []string{"en", "ja", "zh-Hant", "zh-Hans"}

	for _, lang := range langs {
		_ = os.Setenv("MYGO_LOCALE", lang)

		// 1. Narrow main window (680×580)
		a := newTestApp()
		a.settings = defaultSettings()
		tr := music.Track{ID: "test", Name: "Shape of You", Artists: []music.Artist{{Name: "Ed Sheeran"}}, Duration: 233000}
		a.pl.Cue([]music.Track{tr})
		a.pl.Seek(30 * time.Second)
		tt := ui.NewTester(a.view, 680, 580)
		tt.Frame()
		savePNG("main-680x580-"+lang+".png", tt.Image())

		// 2. Settings view (560×720)
		aSettings := newTestApp()
		aSettings.settings = defaultSettings()
		aSettings.settings.Language = lang
		ttSettings := ui.NewTester(aSettings.settingsView, 560, 720)
		ttSettings.Frame()
		savePNG("settings-560x720-"+lang+".png", ttSettings.Image())

		// 3. Settings at min width (480×600)
		ttSettingsNarrow := ui.NewTester(aSettings.settingsView, 480, 600)
		ttSettingsNarrow.Frame()
		savePNG("settings-480x600-"+lang+".png", ttSettingsNarrow.Image())

		// 4. Onboarding dialog
		aOnboard := newTestApp()
		aOnboard.settings = defaultSettings()
		aOnboard.openOnboarding()
		ttOnboard := ui.NewTester(aOnboard.view, 1000, 700)
		ttOnboard.Frame()
		savePNG("onboarding-"+lang+".png", ttOnboard.Image())

		// 5. Now Playing
		aNP := newTestApp()
		aNP.settings = defaultSettings()
		aNP.pl.Cue([]music.Track{tr})
		aNP.nowPlaying = true
		ttNP := ui.NewTester(aNP.view, 1000, 700)
		ttNP.Frame()
		savePNG("nowplaying-"+lang+".png", ttNP.Image())
	}

	_ = os.Setenv("MYGO_LOCALE", "zh-Hans")
}
