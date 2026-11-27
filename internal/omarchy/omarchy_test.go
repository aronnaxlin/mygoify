package omarchy

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The colors of a theme are read as Omarchy writes them: hex, Hyprland's
// rgba(rrggbbaa), CSS's rgb(r, g, b) and the first of a gradient.
func TestParseColor(t *testing.T) {
	for _, c := range []struct {
		in   string
		want ui.Color
	}{
		{"#a39fc7", ui.Hex("#a39fc7")},
		{"#abc", ui.Hex("#aabbcc")},
		{"rgba(a39fc7ff)", ui.Hex("#a39fc7")},
		{"rgba(33ccffee) rgba(00ff99ee) 45deg", ui.RGBA(0x33, 0xcc, 0xff, float32(0xee)/255)},
		{"rgb(13, 12, 21)", ui.Hex("#0d0c15")},
	} {
		got, ok := parseColor(c.in)
		if !ok || got != c.want {
			t.Errorf("%q: %v, %v; want %v", c.in, got, ok, c.want)
		}
	}
	if _, ok := parseColor("blue"); ok {
		t.Error("read a color's name")
	}
}

// A theme takes its popups' colors from shell.toml, following a token that
// names another, and its border from Hyprland's active border.
func TestReadTheme(t *testing.T) {
	colors := parse([]byte(`
mode = "dark"
accent = "#a39fc7"
background = "#0d0c15"
foreground = "#d8c3d6" # the text
`))
	shell := parse([]byte(`
[hyprland]
active-border = "#112233"
[popups]
background = "#010203"
border = "hyprland.active-border"
[controls]
hover-cursor-fill-alpha = 0.5
`))
	th := read(colors, shell, "MesloLGL Nerd Font", 0, 2)
	if th.Background != ui.Hex("#010203") || th.Text != ui.Hex("#d8c3d6") || !th.Dark {
		t.Errorf("background %v, text %v, dark %v", th.Background, th.Text, th.Dark)
	}
	if th.Border != ui.Hex("#112233") || th.BorderWidth != 2 || th.Rounding != 0 {
		t.Errorf("border %v %d, rounding %d", th.Border, th.BorderWidth, th.Rounding)
	}
	if want := ui.Hex("#010203").Mix(ui.Hex("#d8c3d6"), 0.5); th.Hover != want {
		t.Errorf("hover %v, want %v", th.Hover, want)
	}
	// Without shell.toml, an older theme's: colors.toml's background, and
	// the accent for the border.
	old := read(colors, nil, "", 0, 2)
	if old.Background != ui.Hex("#0d0c15") || old.Border != ui.Hex("#a39fc7") {
		t.Errorf("without shell.toml: background %v, border %v", old.Background, old.Border)
	}
}

// A theme that doesn't say whether it is light is told by its background.
func TestReadLightTheme(t *testing.T) {
	th := read(parse([]byte(`background = "#f5f5f5"
foreground = "#222222"`)), nil, "", 0, 0)
	if th.Dark {
		t.Error("a light background read as a dark theme")
	}
}
