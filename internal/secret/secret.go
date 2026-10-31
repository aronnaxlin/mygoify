// Package secret keeps the listener's secrets: their Spotify login, whose
// refresh token signs them in for good, and Soloist's API key (DESIGN.md
// §4.2). The settings file keeps the rest, which is no secret.
package secret

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// The secrets the app keeps.
const (
	SpotifyLogin = "spotify-login" // the Web API's token, as auth stores it
	SoloistKey   = "soloist-api-key"
)

// ErrNotFound is what Get returns for a secret not kept.
var ErrNotFound = errors.New("secret: not found")

// Store keeps secrets by name. Its methods are safe from any goroutine.
type Store interface {
	Get(name string) ([]byte, error)
	Set(name string, v []byte) error
	// Delete forgets a secret; one not kept is no error.
	Delete(name string) error
}

// Default is the store the app keeps its secrets in, with dir for what
// it keeps in files.
func Default(dir string) Store { return Files(dir) }

// Files keeps each secret in a file of its own in the directory, which
// only the listener can read: other users of the computer can't, though
// the listener's programs can.
type Files string

func (d Files) Get(name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(string(d), name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

func (d Files) Set(name string, v []byte) error {
	if err := os.MkdirAll(string(d), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(string(d), name), v, 0o600)
}

func (d Files) Delete(name string) error {
	err := os.Remove(filepath.Join(string(d), name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Entry is one secret of a store, as auth.Session keeps its token.
type Entry struct {
	Store Store
	Name  string
}

func (e Entry) Load() ([]byte, error) { return e.Store.Get(e.Name) }
func (e Entry) Save(b []byte) error   { return e.Store.Set(e.Name, b) }
func (e Entry) Delete() error         { return e.Store.Delete(e.Name) }
