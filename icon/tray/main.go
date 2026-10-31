// Command tray draws internal/app/tray/tray-template.png, the app's icon in
// the macOS menu bar: the app's mark (icon/mark), black on transparent for
// macOS to tint, supersampled, at 2× of the menu bar's 18 points.
//
// Run it from the repository root:
//
//	go run ./icon/tray
//
// With -preview dir, it also writes the icon over light and dark menu bars,
// enlarged, to check by eye.
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"mygoify/icon/mark"
)

// inside tells whether the point x, y of the mark's 1024-point canvas is
// inside the mark.
func inside(x, y float64) bool {
	ux, uy := mark.Upright(x, y)
	for i := range mark.Tops {
		if mark.InStroke(i, ux, uy) || mark.InDot(i, ux, uy) {
			return true
		}
	}
	return false
}

// The mark's longer side, centred in the 18-point square.
const fit = 15.6

// frame is the scale and offset from the square to the canvas, which fit
// the mark's bounds to the square.
func frame() (scale, offX, offY float64) {
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for y := 0.0; y < 1024; y++ {
		for x := 0.0; x < 1024; x++ {
			if inside(x, y) {
				x0, y0, x1, y1 = math.Min(x0, x), math.Min(y0, y), math.Max(x1, x), math.Max(y1, y)
			}
		}
	}
	scale = fit / math.Max(x1-x0, y1-y0)
	return scale, (x0+x1)/2 - 9/scale, (y0+y1)/2 - 9/scale
}

// render draws the icon px pixels square, each pixel's alpha the share of
// 16×16 samples inside the mark.
func render(px int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	s := float64(px) / 18
	k, offX, offY := frame()
	const n = 16
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			hit := 0
			for j := 0; j < n; j++ {
				for i := 0; i < n; i++ {
					if inside((float64(x)+(float64(i)+0.5)/n)/s/k+offX, (float64(y)+(float64(j)+0.5)/n)/s/k+offY) {
						hit++
					}
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, uint8(math.Round(float64(hit) * 255 / (n * n)))})
		}
	}
	return img
}

// preview puts the icon on a light menu bar and a dark one, tinted as
// macOS tints it, and enlarged with nearest pixels so each one shows.
func preview(icon *image.NRGBA, zoom int) *image.NRGBA {
	w := icon.Bounds().Dx()
	pad := w / 3
	side := w + 2*pad
	out := image.NewNRGBA(image.Rect(0, 0, side*2*zoom, side*zoom))
	grounds := []color.NRGBA{{236, 236, 238, 255}, {40, 40, 44, 255}}
	inks := []float64{0, 255}
	for g := range grounds {
		for y := 0; y < side; y++ {
			for x := 0; x < side; x++ {
				c := grounds[g]
				if ix, iy := x-pad, y-pad; ix >= 0 && iy >= 0 && ix < w && iy < w {
					a := float64(icon.NRGBAAt(ix, iy).A) / 255
					mix := func(v uint8) uint8 { return uint8(float64(v)*(1-a) + inks[g]*a) }
					c = color.NRGBA{mix(c.R), mix(c.G), mix(c.B), 255}
				}
				for dy := 0; dy < zoom; dy++ {
					for dx := 0; dx < zoom; dx++ {
						out.SetNRGBA((g*side+x)*zoom+dx, y*zoom+dy, c)
					}
				}
			}
		}
	}
	return out
}

func save(name string, img image.Image) {
	f, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	out := flag.String("o", "internal/app/tray/tray-template.png", "where to write the icon")
	dir := flag.String("preview", "", "a directory for previews")
	flag.Parse()
	icon := render(36)
	save(*out, icon)
	if *dir != "" {
		save(filepath.Join(*dir, "preview-2x.png"), preview(icon, 8))
		save(filepath.Join(*dir, "preview-1x.png"), preview(render(18), 12))
		save(filepath.Join(*dir, "preview-large.png"), preview(render(360), 1))
	}
}
