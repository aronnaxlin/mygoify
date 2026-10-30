package music

import "testing"

// Images.URL picks the smallest picture large enough, else the largest,
// whatever order Spotify lists them in.
func TestImagesURL(t *testing.T) {
	im := Images{{"640", 640, 640}, {"64", 64, 64}, {"300", 300, 300}}
	for _, c := range []struct {
		px   int
		want string
	}{{48, "64"}, {64, "64"}, {65, "300"}, {300, "300"}, {500, "640"}, {2000, "640"}} {
		if got := im.URL(c.px); got != c.want {
			t.Errorf("URL(%d) = %q, want %q", c.px, got, c.want)
		}
	}
	if got := (Images{{URL: "mosaic"}}).URL(300); got != "mosaic" {
		t.Errorf("a picture of unknown size: %q", got)
	}
	if got := Images(nil).URL(300); got != "" {
		t.Errorf("no pictures: %q", got)
	}
}
