package soloist

import (
	"testing"
	"time"
)

// Soloist says when it was built, and 90 days on from then it expires: a
// fortnight before, the app starts saying so.
func TestVersionFromSoloistsOwnWord(t *testing.T) {
	out := "soloist 1.3.10.31 build 1793361706 (20261030) (abcdef) (linux/x86_64)\n"
	v, err := ParseVersion(out)
	if err != nil {
		t.Fatal(err)
	}
	if v.Number != "1.3.10.31" {
		t.Errorf("number %q", v.Number)
	}
	built := time.Date(2026, 10, 30, 12, 1, 46, 0, time.UTC)
	if !v.Build.Equal(built) {
		t.Errorf("built %v, want %v", v.Build, built)
	}
	expires := built.Add(90 * 24 * time.Hour)
	if !v.Expires().Equal(expires) {
		t.Errorf("expires %v, want %v", v.Expires(), expires)
	}
	for _, c := range []struct {
		at    time.Time
		early bool
	}{
		{expires.Add(-90 * 24 * time.Hour), false}, // the day it was built
		{expires.Add(-Remind - time.Minute), false},
		{expires.Add(-Remind), true},
		{expires.Add(-time.Hour), true},
		{expires, false}, // past: to be updated, not reminded of
		{expires.Add(time.Minute), false},
	} {
		if got := v.Expiring(c.at); got != c.early {
			t.Errorf("expiring %v before expiry: %v, want %v", expires.Sub(c.at), got, c.early)
		}
	}
}

// A path in front of its name, or a build number of another length, is
// still Soloist's own word.
func TestVersionOfAnyPath(t *testing.T) {
	v, err := ParseVersion("/home/someone/.local/share/mygoify/soloist-bin/soloist 1.4 build 1800000000 (20261201) (x) (linux/x86_64)\n")
	if err != nil {
		t.Fatal(err)
	}
	if v.Number != "1.4" || !v.Build.Equal(time.Unix(1800000000, 0)) {
		t.Errorf("%+v", v)
	}
}

// What doesn't say when it was built is no version at all.
func TestVersionWithoutABuild(t *testing.T) {
	for _, out := range []string{"", "soloist 1.3.10.31\n", "build 12\n", "soloist 1.3.10.31 build abc (20261030)\n"} {
		if _, err := ParseVersion(out); err == nil {
			t.Errorf("%q has a version", out)
		}
	}
}
