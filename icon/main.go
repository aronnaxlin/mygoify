// Command icon draws the app icons from the app's mark (icon/mark): the
// layers of AppIcon.icon, the Icon Composer document that macOS 26 and later
// render in Liquid Glass, and resources/icon.png, the same icon as macOS
// draws it, on the grid of older macOS icons, for older macOS, Windows and
// Linux; from AppIcon-dev.icon, the strokes outlined on paper, or on deep
// green in the dark, resources/icon-dev.png, the icon of mygo dev builds;
// and from them the icon in the notification area.
//
// Run it from the repository root on a Mac with Xcode 26 or later:
//
//	go run ./icon
//
// The icon.json of each document, its background, glass and the colors of
// its layers in light and dark, is edited by hand or in Icon Composer.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mygoify/icon/mark"
)

// icons are the documents drawn, with the PNG rendered from each, and
// whether the strokes are outlined.
var icons = []struct {
	document, output string
	outlined         bool
}{
	{"icon/AppIcon.icon", "resources/icon.png", false},
	{"icon/AppIcon-dev.icon", "resources/icon-dev.png", true},
}

func main() {
	tool, err := ictool()
	if err != nil {
		log.Fatal(err)
	}
	for _, ic := range icons {
		// The layers are black: icon.json fills them, in each appearance.
		layers := map[string]string{
			"strokes.svg": svg(mark.Strokes("#000000", ic.outlined)),
			"dots.svg":    svg(mark.Dots("#000000")),
		}
		for name, s := range layers {
			if err := os.WriteFile(filepath.Join(ic.document, "Assets", name), []byte(s), 0o644); err != nil {
				log.Fatal(err)
			}
		}
		if err := render(tool, ic.document, ic.output); err != nil {
			log.Fatal(err)
		}
	}
	// The notification area shows the icon small, as it is.
	if err := copyFile("resources/icon.png", "internal/app/tray/tray.png"); err != nil {
		log.Fatal(err)
	}
	if out, err := exec.Command("sips", "-z", "64", "64", "internal/app/tray/tray.png").CombinedOutput(); err != nil {
		log.Fatalf("sips: %v\n%s", err, out)
	}
}

func svg(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">` + "\n" + body + "\n</svg>\n"
}

// ictool is Icon Composer's command-line tool, in the Xcode that
// xcode-select picks.
func ictool() (string, error) {
	out, err := exec.Command("xcode-select", "-p").Output()
	if err != nil {
		return "", fmt.Errorf("xcode-select (Xcode 26 or later draws the icons): %v", err)
	}
	developer := strings.TrimSpace(string(out))
	return filepath.Join(developer, "..", "Applications", "Icon Composer.app", "Contents", "Executables", "ictool"), nil
}

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}

// render has ictool draw document as macOS does, and lays it on the
// 1024-pixel grid of macOS icons at output: an 824-pixel body, centered,
// over a soft shadow.
func render(tool, document, output string) error {
	const canvas, body = 1024, 824
	dir, err := os.MkdirTemp("", "icon")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	drawn := filepath.Join(dir, "icon.png")
	size := fmt.Sprint(body)
	if out, err := exec.Command(tool, document, "--export-image", "--output-file", drawn,
		"--platform", "macOS", "--rendition", "Default", "--width", size, "--height", size, "--scale", "1").CombinedOutput(); err != nil {
		return fmt.Errorf("ictool (it comes with Xcode 26 or later): %v\n%s", err, out)
	}
	// ictool draws in Display P3, and the PNG this writes carries no
	// profile, which readers take as sRGB: convert, or the greens go dull.
	if out, err := exec.Command("sips", "--matchTo", "/System/Library/ColorSync/Profiles/sRGB Profile.icc", drawn).CombinedOutput(); err != nil {
		return fmt.Errorf("sips: %v\n%s", err, out)
	}
	f, err := os.Open(drawn)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return err
	}

	at := image.Pt((canvas-body)/2, (canvas-body)/2)
	dst := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	shadow := image.NewAlpha(dst.Bounds())
	draw.Draw(shadow, src.Bounds().Add(at).Add(image.Pt(0, 12)), src, src.Bounds().Min, draw.Src)
	blur(shadow, 10)
	draw.DrawMask(dst, dst.Bounds(), image.NewUniform(color.NRGBA{A: 72}), image.Point{}, shadow, image.Point{}, draw.Over)
	draw.Draw(dst, src.Bounds().Add(at), src, src.Bounds().Min, draw.Over)

	out, err := os.Create(output)
	if err != nil {
		return err
	}
	if err := png.Encode(out, dst); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// blur approximates a Gaussian blur of the given radius with three box
// blurs in each direction.
func blur(m *image.Alpha, radius int) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	buf := make([]uint8, len(m.Pix))
	pass := func(src, dst []uint8, count, length, stride, step int) {
		for line := range count {
			base := line * stride
			sum := 0
			for i := -radius; i <= radius; i++ {
				sum += int(src[base+min(max(i, 0), length-1)*step])
			}
			for i := range length {
				dst[base+i*step] = uint8(sum / (2*radius + 1))
				sum += int(src[base+min(i+radius+1, length-1)*step]) - int(src[base+max(i-radius, 0)*step])
			}
		}
	}
	for range 3 {
		pass(m.Pix, buf, h, w, m.Stride, 1)
		pass(buf, m.Pix, w, h, 1, m.Stride)
	}
}
