package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Naming the device keeps the name, which the next start writes into
// Soloist's service: the pairing is the device's ID, not its name, so it
// stays.
func TestSettingsDeviceName(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("填写设备名"); err != nil {
		t.Fatal(err)
	}
	tt.Type("书房")
	tt.Frame()
	if got := a.settings.Spotify.DeviceName; got != "书房" {
		t.Fatalf("device name %q", got)
	}
}

// Choosing a language changes the language shown by the app, and keeps the
// choice in the locale file and in settings.json.
func TestSettingsLanguage(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() {
		mygo.App.SetPath(mygo.PathUserData, "")
		_ = os.Setenv("MYGO_LOCALE", "zh-Hans")
		mygo.App.SetLocale("zh-Hans")
	})

	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(dir, "settings.json")

	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()

	// Initially in zh-Hans:
	if _, ok := tt.Find("外观"); !ok {
		t.Fatal("expected to find 外观 in initial view")
	}

	// Click language select to choose English:
	if err := tt.Click("选择语言"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("English"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if a.settings.Language != "en" {
		t.Fatalf("settings.Language = %q, want en", a.settings.Language)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "locale")); err != nil || strings.TrimSpace(string(data)) != "en" {
		t.Fatalf("locale file = %q (%v), want en", data, err)
	}
	if _, ok := tt.Find("Appearance"); !ok {
		t.Fatal("expected to find Appearance after switching to English")
	}

	// Switch to Japanese:
	if err := tt.Click("Select language"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("日本語"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if a.settings.Language != "ja" {
		t.Fatalf("settings.Language = %q, want ja", a.settings.Language)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "locale")); err != nil || strings.TrimSpace(string(data)) != "ja" {
		t.Fatalf("locale file = %q (%v), want ja", data, err)
	}
	if _, ok := tt.Find("外観"); !ok {
		t.Fatal("expected to find 外観 after switching to Japanese")
	}

	// Switch to Follow System:
	r, ok := tt.Find("言語を選択")
	if !ok {
		t.Fatal("expected to find 言語を選択")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	// Click the first item in the popup (システムに従う) directly by position:
	tt.ClickAt(r.X+r.W/2, r.Y+r.H+20)
	tt.Frame()

	if a.settings.Language != "" {
		t.Fatalf("settings.Language = %q, want empty", a.settings.Language)
	}
	if _, err := os.Stat(filepath.Join(dir, "locale")); !os.IsNotExist(err) {
		t.Fatalf("locale file should be removed for empty tag, got %v", err)
	}
	// With no system language matching zh, following system falls back to English:
	if _, ok := tt.Find("Appearance"); !ok {
		t.Fatal("expected to find Appearance after switching back to system fallback")
	}
}
