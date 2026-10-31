// Package mark is the app's mark: five exclamation marks that stand as the
// bars of an equalizer, their strokes of different heights over dots on one
// line, the whole leaning forward a little. It is laid out on the
// 1024-point canvas of the app's icon, which icon draws, and icon/tray draws
// it again for the menu bar.
package mark

import (
	"fmt"
	"math"
	"strings"
)

// A stroke is round at its top, of radius TopR, and narrows to a round foot
// of radius FootR, its centre at Foot. The columns of the marks are Pitch
// apart, the first's centre at First.
const (
	TopR, FootR = 43.0, 31.0
	Foot        = 642.0
	Pitch       = 122.0
	First       = 268.0

	// The dots sit on one line below the strokes.
	DotR = 44.0
	DotY = 757.0

	// The mark leans this many degrees, about the height Pivot, where it
	// balances, so that leaning keeps it centred on the canvas.
	Lean  = 5.0
	Pivot = 600.0

	// The width of the outline of the dev icon's strokes.
	Outline = 22.0
)

// Tops are the centres of the strokes' tops, from the left: the levels of
// the equalizer.
var Tops = [5]float64{486, 326, 416, 266, 446}

// X is the centre of the i-th column.
func X(i int) float64 { return First + Pitch*float64(i) }

// slant is how far x moves right for each point up.
func slant() float64 { return math.Tan(Lean * math.Pi / 180) }

// Upright moves a point of the leaning mark back to the upright one.
func Upright(x, y float64) (float64, float64) { return x - slant()*(Pivot-y), y }

// Transform is the SVG transform that leans the upright mark.
func Transform() string {
	t := slant()
	return fmt.Sprintf("matrix(1 0 %s 1 %s 0)", n(-t), n(t*Pivot))
}

// InStroke tells whether the upright point x, y is inside the i-th stroke:
// its round top, its round foot or the tapering side between them, as
// StrokePath draws it.
func InStroke(i int, x, y float64) bool {
	cx, top := X(i), Tops[i]
	switch {
	case y <= top:
		return math.Hypot(x-cx, y-top) < TopR
	case y >= Foot:
		return math.Hypot(x-cx, y-Foot) < FootR
	}
	half := TopR + (FootR-TopR)*(y-top)/(Foot-top)
	return math.Abs(x-cx) < half
}

// InDot tells whether the upright point x, y is inside the i-th dot.
func InDot(i int, x, y float64) bool { return math.Hypot(x-X(i), y-DotY) < DotR }

// StrokePath is the SVG path of the i-th stroke, upright.
func StrokePath(i int) string {
	cx, top := X(i), Tops[i]
	return fmt.Sprintf("M%s %sA%s %s 0 0 1 %s %sL%s %sA%s %s 0 0 1 %s %sZ",
		n(cx-TopR), n(top), n(TopR), n(TopR), n(cx+TopR), n(top),
		n(cx+FootR), n(Foot), n(FootR), n(FootR), n(cx-FootR), n(Foot))
}

// Strokes is the SVG of the five strokes, leaning, filled with fill or,
// when outlined, drawn as outlines of that color.
func Strokes(fill string, outlined bool) string {
	var b strings.Builder
	paint := fmt.Sprintf(`fill="%s"`, fill)
	if outlined {
		paint = fmt.Sprintf(`fill="none" stroke="%s" stroke-width="%s" stroke-linejoin="round"`, fill, n(Outline))
	}
	fmt.Fprintf(&b, `<g transform="%s" %s>`, Transform(), paint)
	for i := range Tops {
		fmt.Fprintf(&b, "\n"+`<path d="%s"/>`, StrokePath(i))
	}
	b.WriteString("\n</g>")
	return b.String()
}

// Dots is the SVG of the five dots, leaning, filled with fill.
func Dots(fill string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<g transform="%s" fill="%s">`, Transform(), fill)
	for i := range Tops {
		fmt.Fprintf(&b, "\n"+`<circle cx="%s" cy="%s" r="%s"/>`, n(X(i)), n(DotY), n(DotR))
	}
	b.WriteString("\n</g>")
	return b.String()
}

func n(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.3f", v), "0"), ".")
}
