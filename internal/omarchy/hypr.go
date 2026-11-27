package omarchy

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Hyprland reports whether the app runs under Hyprland, Omarchy's
// compositor or anyone's. A Wayland window can't place itself: a panel
// that is to drop from the bar asks Hyprland to put it there.
func Hyprland() bool {
	return runtime.GOOS == "linux" && os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != ""
}

// PanelGap is how far a drop-down panel keeps from the bar and the
// screen's edges, as Omarchy's own do.
const PanelGap = 6

// monitor is a monitor as hyprctl monitors describes it.
type monitor struct {
	Name          string
	X, Y          int
	Width, Height int
	Scale         float64
	Transform     int
	Reserved      [4]int // left, top, right, bottom: the bar's room
}

// size is the monitor's size in the coordinates windows are placed in.
func (m monitor) size() (w, h int) {
	s := m.Scale
	if s <= 0 {
		s = 1
	}
	w, h = int(float64(m.Width)/s+.5), int(float64(m.Height)/s+.5)
	if m.Transform%2 == 1 { // turned a quarter
		w, h = h, w
	}
	return w, h
}

// Spot is where a panel goes: on monitor Monitor, at X, Y from its top
// left corner, with MaxHeight down to the bottom of its work area.
type Spot struct {
	Monitor   string
	X, Y      int
	MaxHeight int
	// Left and Top are X and Y across all monitors, as the cursor is.
	Left, Top int
}

// PanelAt is where a panel width wide drops from the bar: under it, its
// middle at the cursor, which has just clicked the icon it drops from,
// kept PanelGap inside the monitor the cursor is on.
func PanelAt(width int) (Spot, bool) {
	var cur struct{ X, Y int }
	var mons []monitor
	if HyprJSON(&cur, "cursorpos") != nil || HyprJSON(&mons, "monitors") != nil {
		return Spot{}, false
	}
	return panelAt(width, cur.X, cur.Y, mons)
}

func panelAt(width, cx, cy int, mons []monitor) (Spot, bool) {
	for _, m := range mons {
		w, h := m.size()
		if cx < m.X || cx >= m.X+w || cy < m.Y || cy >= m.Y+h {
			continue
		}
		left, right := m.Reserved[0]+PanelGap, w-m.Reserved[2]-PanelGap
		x := max(left, min(right-width, cx-m.X-width/2))
		y := m.Reserved[1] + PanelGap
		return Spot{Monitor: m.Name, X: x, Y: y, MaxHeight: h - m.Reserved[3] - PanelGap - y, Left: m.X + x, Top: m.Y + y}, true
	}
	return Spot{}, false
}

// PlacePanel has Hyprland put the window titled title at s, floating on
// every workspace, without its border or its shadow, fully opaque: the
// panel draws its own frame. A rule replaces the last one of its name; a
// reload of Hyprland's config drops it.
func PlacePanel(title string, s Spot) error {
	match := "^" + regexp.QuoteMeta(title) + "$"
	// Hyprland configured in Lua, as Omarchy 4's is, takes a rule as Lua.
	on := ""
	if s.Monitor != "" {
		on = "monitor = " + strconv.Quote(s.Monitor) + ", "
	}
	lua := fmt.Sprintf(`hl.window_rule({ name = "mygoify-panel", match = { title = %s }, tag = "-default-opacity", float = true, pin = true, border_size = 0, no_shadow = true, opacity = "1 1", no_anim = true, %smove = { %d, %d } })`,
		strconv.Quote(match), on, s.X, s.Y)
	if hyprctl("eval", lua) == nil {
		return nil
	}
	// Else its classic config's keywords.
	rule := "title:" + match
	var batch []string
	for _, r := range []string{"float", "pin", "noborder", "noshadow", "noanim", "opaque", fmt.Sprintf("move %d %d", s.Left, s.Top)} {
		batch = append(batch, "keyword windowrulev2 "+r+", "+rule)
	}
	return hyprctl("--batch", strings.Join(batch, " ; "))
}

// clickEvent is what Hyprland announces of each left click while a panel
// watches for clicks outside it. Omarchy's focus follows the mouse, so a
// panel can't close as it loses focus: the pointer passing over another
// window takes it. A bind of the left button that lets the click through
// announces each on Hyprland's event socket instead.
const clickEvent = "mygoify-click"

// WatchClicks calls click on each left click, anywhere, until stop is
// called, which takes the bind away again.
func WatchClicks(click func()) (stop func()) {
	if hyprctl("eval", `if mygoify_click then mygoify_click:remove() end mygoify_click = hl.bind("mouse:272", function() hl.dispatch(hl.dsp.event("`+clickEvent+`")) end, { non_consuming = true })`) != nil {
		hyprctl("keyword", "bindn", ", mouse:272, event, "+clickEvent)
	}
	sock := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "hypr", os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"), ".socket2.sock")
	c, err := net.Dial("unix", sock)
	if err == nil {
		go func() {
			sc := bufio.NewScanner(c)
			for sc.Scan() {
				if sc.Text() == "custom>>"+clickEvent {
					click()
				}
			}
		}()
	}
	return func() {
		if c != nil {
			c.Close()
		}
		if hyprctl("eval", `if mygoify_click then mygoify_click:remove() mygoify_click = nil end`) != nil {
			hyprctl("keyword", "unbind", ", mouse:272")
		}
	}
}

// Cursor is where the pointer is, across all monitors.
func Cursor() (x, y int, ok bool) {
	var cur struct{ X, Y int }
	if HyprJSON(&cur, "cursorpos") != nil {
		return 0, 0, false
	}
	return cur.X, cur.Y, true
}

func hyprctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "hyprctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("hyprctl %s: %v: %s", args[0], err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" && s != "ok" && !strings.HasPrefix(s, "ok") {
		return fmt.Errorf("hyprctl %s: %s", args[0], s)
	}
	return nil
}
