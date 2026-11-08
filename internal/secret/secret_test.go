package secret

import (
	"errors"
	"testing"
)

// A secret kept in a file moves into the keychain as it is first read,
// and the file goes: a secret is kept once.
func TestMoving(t *testing.T) {
	to, from := Files(t.TempDir()), Files(t.TempDir())
	from.Set(SoloistKey, []byte("k"))
	m := moving{to, from}
	if v, err := m.Get(SoloistKey); err != nil || string(v) != "k" {
		t.Fatalf("Get = %q, %v; want the file's", v, err)
	}
	if v, _ := to.Get(SoloistKey); string(v) != "k" {
		t.Errorf("keychain keeps %q, want the file's", v)
	}
	if _, err := from.Get(SoloistKey); !errors.Is(err, ErrNotFound) {
		t.Errorf("the file is left, err %v", err)
	}
	if _, err := m.Get(SpotifyLogin); !errors.Is(err, ErrNotFound) {
		t.Errorf("a secret kept nowhere: err %v, want ErrNotFound", err)
	}
}

// Choosing files over the keychain, and back, signs no one out: the
// login moves to the store chosen as the app first reads it, and is kept
// once, there.
func TestSwitchingKeepsTheLogin(t *testing.T) {
	sys, files := Files(t.TempDir()), Files(t.TempDir())
	open(sys, files, false).Set(SpotifyLogin, []byte("token"))
	for _, c := range []struct {
		inFiles     bool
		kept, other Store
	}{{true, files, sys}, {false, sys, files}} {
		if v, err := open(sys, files, c.inFiles).Get(SpotifyLogin); err != nil || string(v) != "token" {
			t.Fatalf("in files %v: Get = %q, %v", c.inFiles, v, err)
		}
		if v, _ := c.kept.Get(SpotifyLogin); string(v) != "token" {
			t.Errorf("in files %v: the store chosen keeps %q", c.inFiles, v)
		}
		if _, err := c.other.Get(SpotifyLogin); !errors.Is(err, ErrNotFound) {
			t.Errorf("in files %v: the other store kept it too, err %v", c.inFiles, err)
		}
	}
}
