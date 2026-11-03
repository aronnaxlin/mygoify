package app

import (
	"testing"

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
