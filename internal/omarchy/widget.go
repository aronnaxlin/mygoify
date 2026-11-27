package omarchy

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mygoify/icon/mark"
)

// The bar widget is MyGoify's icon in Omarchy's own bar, where Omarchy's
// widgets are: the app's mark, five exclamation marks, in the bar's color.
// A click opens the mini player under it, a right click the app. It is a
// plugin of Omarchy's shell, put in as its README says: its files in
// ~/.config/omarchy/plugins/<id>/, then rescanPlugins and plugin enable,
// as magpie puts its own.
const WidgetID = "aronnaxlin.mygoify"

func widgetDir() string {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".config")
	}
	return filepath.Join(cfg, "omarchy", "plugins", WidgetID)
}

const manifest = `{
  "schemaVersion": 1,
  "id": "` + WidgetID + `",
  "name": "MyGoify",
  "version": "1.0.0",
  "author": "MyGoify",
  "license": "AGPL-3.0",
  "description": "MyGoify's icon in the bar: a click opens its mini player under it, a right click its window.",
  "kinds": ["bar-widget"],
  "entryPoints": {
    "barWidget": "Widget.qml"
  },
  "barWidget": {
    "displayName": "MyGoify",
    "description": "What plays on Spotify: a click opens MyGoify's mini player, a right click its window.",
    "category": "Media",
    "aliases": ["mygoify", "spotify"],
    "allowMultiple": false,
    "defaultSection": "right"
  }
}
`

// widgetQML is the widget, running exe: __EXE__ is its path, quoted as
// QML quotes, and __PATH__ and __W__, __H__ the mark's outline and size.
const widgetQML = `import QtQuick
import QtQuick.Shapes
import Quickshell
import qs.Commons
import qs.Ui

// MyGoify's icon in Omarchy's bar, put here by MyGoify. A click opens its
// mini player under the icon, a right click its window.
BarWidget {
  id: root
  moduleName: "` + WidgetID + `"

  // written in by MyGoify as it puts the widget here
  readonly property string mygoify: __EXE__

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    tooltipText: "MyGoify"
    // The app's mark, five exclamation marks, in the bar's color.
    iconComponent: Component {
      Shape {
        width: __W__
        height: __H__
        preferredRendererType: Shape.CurveRenderer
        ShapePath {
          strokeWidth: -1
          fillColor: button.foreground
          PathSvg { path: "__PATH__" }
        }
      }
    }
    onPressed: function(b) {
      Quickshell.execDetached(b === Qt.RightButton ? [root.mygoify] : [root.mygoify, "panel"])
    }
  }
}
`

// markOutline is the mark's outline as SVG path data, leaning as it does,
// from its top left corner, and its size: the strokes and the dots, each a
// polygon fine enough to look round at the size of a bar's icon.
func markOutline() (string, float64, float64) {
	t := math.Tan(mark.Lean * math.Pi / 180)
	lean := func(x, y float64) (float64, float64) { return x + t*(mark.Pivot-y), y }
	var shapes [][][2]float64
	arc := func(cx, cy, r, from, to float64, n int) [][2]float64 {
		var pts [][2]float64
		for k := 0; k <= n; k++ {
			a := from + (to-from)*float64(k)/float64(n)
			pts = append(pts, [2]float64{cx + r*math.Cos(a), cy + r*math.Sin(a)})
		}
		return pts
	}
	for i, top := range mark.Tops {
		cx := mark.X(i)
		// The round top, left to right over it, then the round foot.
		s := arc(cx, top, mark.TopR, math.Pi, 2*math.Pi, 16)
		s = append(s, arc(cx, mark.Foot, mark.FootR, 0, math.Pi, 12)...)
		shapes = append(shapes, s)
		shapes = append(shapes, arc(cx, mark.DotY, mark.DotR, 0, 2*math.Pi, 24))
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, s := range shapes {
		for k, p := range s {
			x, y := lean(p[0], p[1])
			s[k] = [2]float64{x, y}
			minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
		}
	}
	var b strings.Builder
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
	for _, s := range shapes {
		for k, p := range s {
			cmd := "L"
			if k == 0 {
				cmd = "M"
			}
			b.WriteString(cmd + f(p[0]-minX) + " " + f(p[1]-minY))
		}
		b.WriteString("Z")
	}
	return b.String(), math.Ceil(maxX - minX), math.Ceil(maxY - minY)
}

// widgetFiles is the widget's files, running exe.
func widgetFiles(exe string) map[string][]byte {
	path, w, h := markOutline()
	qml := strings.NewReplacer(
		"__EXE__", strconv.Quote(exe),
		"__PATH__", path,
		"__W__", fmt.Sprint(w),
		"__H__", fmt.Sprint(h),
	).Replace(widgetQML)
	return map[string][]byte{"manifest.json": []byte(manifest), "Widget.qml": []byte(qml)}
}

// KeepWidget puts the widget in Omarchy's bar, running exe, or brings the
// one there up to date with it, which may have moved or been updated. It
// does nothing when the widget is there as it would put it.
func KeepWidget(exe string) error {
	dir := widgetDir()
	files := widgetFiles(exe)
	same := true
	for name, b := range files {
		if old, err := os.ReadFile(filepath.Join(dir, name)); err != nil || !bytes.Equal(old, b) {
			same = false
		}
	}
	if same {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return err
		}
	}
	if err := run("omarchy-shell", "shell", "rescanPlugins"); err != nil {
		return err
	}
	// Enabled again, the shell loads the files it had already.
	_ = run("omarchy", "plugin", "disable", WidgetID)
	return run("omarchy", "plugin", "enable", WidgetID)
}

// RemoveWidget takes the widget out of the bar.
func RemoveWidget() error {
	if _, err := os.Stat(widgetDir()); err != nil {
		return nil
	}
	_ = run("omarchy", "plugin", "disable", WidgetID)
	if err := os.RemoveAll(widgetDir()); err != nil {
		return err
	}
	return run("omarchy-shell", "shell", "rescanPlugins")
}

func run(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
