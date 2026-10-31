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
