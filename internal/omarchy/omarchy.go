// Package omarchy is what the app knows of Omarchy (omarchy.org), the
// Hyprland desktop: whether it runs on one, and the look of its current
// theme, so that the mini player looks like Omarchy's own panels there
// (DESIGN.md §4.3). It reads what yetone/magpie reads for the same.
//
// Omarchy 4 keeps the current theme in ~/.local/state/omarchy/current/theme:
// colors.toml is the palette every app's theme is made from, and
// shell.toml the colors and alphas its shell draws controls, popups and
// menus with. The font is omarchy-font-current's, and the corners and the
// border Hyprland's decoration:rounding and general:border_size.
package omarchy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// Detect reports whether the app runs on Omarchy: on Linux, with a theme
// of Omarchy's current. MYGOIFY_OMARCHY=1 says it does, =0 that it doesn't.
func Detect() bool {
	switch os.Getenv("MYGOIFY_OMARCHY") {
	case "1":
		return true
	case "0":
		return false
	}
	if runtime.GOOS != "linux" || ThemeDir() == "" {
		return false
	}
	if os.Getenv("OMARCHY_PATH") != "" {
		return true
	}
	home, _ := os.UserHomeDir()
	for _, d := range []string{"/usr/share/omarchy", filepath.Join(home, ".local", "share", "omarchy")} {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

// ThemeDir is the current theme's directory, or "" without one.
func ThemeDir() string {
	if d := os.Getenv("MYGOIFY_OMARCHY_THEME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	for _, d := range []string{
		filepath.Join(state, "omarchy", "current", "theme"),
		filepath.Join(home, ".config", "omarchy", "current", "theme"), // before Omarchy 4
	} {
		if _, err := os.Stat(filepath.Join(d, "colors.toml")); err == nil {
			return d
		}
	}
	return ""
}

// Theme is Omarchy's current theme, as the app draws with it.
type Theme struct {
	Dark bool
	// Background and Text are those of Omarchy's popups.
	Background, Text ui.Color
	Muted, Faint     ui.Color
	// Hover and Press fill a control under the pointer and pressed, and
	// Selected one chosen, as Omarchy's shell fills its own.
	Hover, Press, Selected ui.Color
	Line                   ui.Color // a hairline between parts
	Accent, OnAccent       ui.Color
	// Border is the color of the frame of a popup, BorderWidth its width
	// and Rounding the radius of its corners, as Hyprland frames windows.
	Border      ui.Color
	BorderWidth int
	Rounding    int
	Font        string
	FontSize    float32 // the shell's base size of type
}

// read makes the theme of colors.toml's and shell.toml's keys, with the
// font, the corners and the border given.
func read(colors, shell map[string]string, font string, rounding, border int) Theme {
	get := func(m map[string]string, fallback ui.Color, keys ...string) ui.Color {
		for _, k := range keys {
			v := m[k]
			// A token of shell.toml may name another: "hyprland.active-border".
			for range 3 {
				if ref, ok := shell[v]; ok {
					v = ref
				}
			}
			if c, ok := parseColor(v); ok {
				return c
			}
		}
		return fallback
	}
	num := func(k string, fallback float64) float32 {
		if f, err := strconv.ParseFloat(shell[k], 64); err == nil {
			return float32(f)
		}
		return float32(fallback)
	}
	bg := get(shell, get(colors, ui.Hex("#1a1b26"), "background"), "popups.background")
	bg.A = 255
	fg := get(shell, get(colors, ui.Hex("#a9b1d6"), "foreground"), "popups.text")
	dark := !strings.EqualFold(colors["mode"], "light")
	if m := strings.ToLower(colors["mode"]); m != "light" && m != "dark" {
		dark = luminance(bg) < 0.5
	}
	over := func(c ui.Color, a float32) ui.Color { return bg.Mix(c, a) }
	t := Theme{
		Dark:        dark,
		Background:  bg,
		Text:        fg,
		Muted:       get(colors, over(fg, 0.6), "dark_foreground"),
		Faint:       over(fg, 0.38),
		Hover:       over(fg, num("controls.hover-cursor-fill-alpha", 0.08)),
		Press:       over(fg, num("controls.pressed-fill-alpha", 0.22)),
		Selected:    over(fg, num("controls.selected-fill-alpha", 0.18)),
		Line:        over(fg, 0.16),
		Accent:      get(colors, ui.Hex("#7aa2f7"), "accent", "blue"),
		OnAccent:    bg,
		BorderWidth: max(border, 0),
		Rounding:    max(rounding, 0),
		Font:        font,
		FontSize:    num("font.base-size", 12),
	}
	if !dark {
		t.Muted = get(colors, over(fg, 0.6), "muted", "dark_foreground")
	}
	t.Border = get(shell, t.Accent, "popups.border", "hyprland.active-border")
	return t
}

// luminance is c's relative luminance, to tell a light theme from a dark
// one that doesn't say.
func luminance(c ui.Color) float32 {
	return (0.2126*float32(c.R) + 0.7152*float32(c.G) + 0.0722*float32(c.B)) / 255
}

// parse reads a TOML file's key = value lines, a section's keys as
// "section.key": the little of TOML the theme's files use (strings,
// numbers, booleans and comments).
func parse(b []byte) map[string]string {
	out := map[string]string{}
	section := ""
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			section = strings.Trim(line, "[] ")
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if strings.HasPrefix(v, `"`) {
			if end := strings.Index(v[1:], `"`); end >= 0 {
				v = v[1 : end+1]
			}
		} else if i := strings.Index(v, "#"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if section != "" {
			k = section + "." + k
		}
		out[k] = v
	}
	return out
}

// parseColor reads #rgb, #rrggbb, #rrggbbaa, Hyprland's rgb(rrggbb) and
// rgba(rrggbbaa), and CSS's rgb(r, g, b) and rgba(r, g, b, a). Of a
// gradient, "rgba(…) rgba(…) 45deg", it reads the first color.
func parseColor(s string) (ui.Color, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if f := strings.Fields(s); len(f) > 1 && !strings.Contains(s, ",") {
		s = f[0]
	}
	hex := func(h string) (ui.Color, bool) {
		switch len(h) {
		case 3:
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		case 6, 8:
		default:
			return ui.Color{}, false
		}
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return ui.Color{}, false
		}
		if len(h) == 6 {
			n = n<<8 | 0xff
		}
		return ui.RGBA(uint8(n>>24), uint8(n>>16), uint8(n>>8), float32(n&0xff)/255), true
	}
	switch {
	case strings.HasPrefix(s, "#"):
		return hex(s[1:])
	case strings.HasPrefix(s, "rgb"):
		open, end := strings.Index(s, "("), strings.LastIndex(s, ")")
		if open < 0 || end < open {
			return ui.Color{}, false
		}
		in := strings.TrimSpace(s[open+1 : end])
		if !strings.Contains(in, ",") {
			return hex(in)
		}
		parts := strings.Split(in, ",")
		if len(parts) < 3 {
			return ui.Color{}, false
		}
		v := [4]float64{0, 0, 0, 1}
		for i, p := range parts[:min(len(parts), 4)] {
			f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if err != nil {
				return ui.Color{}, false
			}
			v[i] = f
		}
		c := func(f float64) uint8 { return uint8(max(0, min(255, f+0.5))) }
		return ui.RGBA(c(v[0]), c(v[1]), c(v[2]), float32(v[3])), true
	}
	return ui.Color{}, false
}

var (
	mu     sync.Mutex
	cached Theme
	have   bool
	seen   string    // the stamp of the files cached was read from
	looked time.Time // when the files were last looked at
	// The font, the corners and the border, asked of omarchy-font-current
	// and hyprctl only as their own files change: setting a theme reloads
	// Hyprland, and hyprctl asked meanwhile can stall.
	desk     string
	font     string
	rounding int
	border   int
)

// themeFiles are the files whose change changes the theme.
func themeFiles(dir string) []string {
	return []string{
		filepath.Join(dir, "colors.toml"),
		filepath.Join(dir, "shell.toml"),
		filepath.Join(filepath.Dir(dir), "theme.name"),
	}
}

// deskFiles are those whose change may change the font, the corners or
// the border: fontconfig's, which omarchy-font-set writes, and Hyprland's.
func deskFiles() []string {
	home, _ := os.UserHomeDir()
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	hypr, _ := filepath.Glob(filepath.Join(cfg, "hypr", "*"))
	return append([]string{filepath.Join(cfg, "fontconfig", "fonts.conf")}, hypr...)
}

func stampOf(files []string) string {
	var b strings.Builder
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", f, fi.ModTime().UnixNano(), fi.Size())
		}
	}
	return b.String()
}

// Current is the theme as it is now, and whether there is one: read again
// only once one of its files changed, which it looks at once a second at
// most.
func Current() (Theme, bool) {
	dir := ThemeDir()
	if dir == "" {
		return Theme{}, false
	}
	mu.Lock()
	defer mu.Unlock()
	if have && time.Since(looked) < time.Second {
		return cached, true
	}
	looked = time.Now()
	stamp, d := stampOf(themeFiles(dir)), stampOf(deskFiles())
	if have && stamp == seen && d == desk {
		return cached, true
	}
	if d != desk || !have {
		font = fontName()
		rounding, border = hyprInt("decoration:rounding", 0), hyprInt("general:border_size", 2)
		desk = d
	}
	colors, _ := os.ReadFile(filepath.Join(dir, "colors.toml"))
	shell, _ := os.ReadFile(filepath.Join(dir, "shell.toml"))
	cached, have, seen = read(parse(colors), parse(shell), font, rounding, border), true, stamp
	return cached, true
}

// fontName is the font Omarchy sets for everything, or "".
func fontName() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "omarchy-font-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hyprInt is an integer option of Hyprland's, or fallback.
func hyprInt(option string, fallback int) int {
	var v struct {
		Int *int `json:"int"`
	}
	if HyprJSON(&v, "getoption", option) != nil || v.Int == nil {
		return fallback
	}
	return *v.Int
}

// HyprJSON asks hyprctl for what as JSON, into v.
func HyprJSON(v any, what ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "hyprctl", append([]string{"-j"}, what...)...).Output()
	if err != nil {
		return err
	}
	return json.Unmarshal(out, v)
}
