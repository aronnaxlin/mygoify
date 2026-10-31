// Package secret keeps the listener's secrets: their Spotify login, whose
// refresh token signs them in for good, and Soloist's API key (DESIGN.md
// §4.2). The settings file keeps the rest, which is no secret.
package secret

import (
	"errors"
	"io/fs"
	"log"
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

// Default is the store the app keeps its secrets in: the system's
// keychain, which takes over the secrets kept in files in dir, or those
// files where there is none. MYGOIFY_SECRETS=files keeps them in files
// anyway, for development: macOS asks the listener to let each new build
// read the keychain, and the app is built anew at each change.
func Default(dir string) Store {
	if os.Getenv("MYGOIFY_SECRETS") == "files" {
		return Files(dir)
	}
	s, err := system()
	if err != nil {
		log.Printf("secret: no keychain, keeping secrets in files: %v", err)
		return Files(dir)
	}
	return moving{s, Files(dir)}
}

// moving keeps secrets in to, and moves there those from keeps as they
// are asked for.
type moving struct{ to, from Store }

func (m moving) Get(name string) ([]byte, error) {
	v, err := m.to.Get(name)
	if !errors.Is(err, ErrNotFound) {
		return v, err
	}
	if v, err = m.from.Get(name); err != nil {
		return nil, err
	}
	if err := m.to.Set(name, v); err != nil {
		return nil, err
	}
	m.from.Delete(name)
	return v, nil
}

func (m moving) Set(name string, v []byte) error {
	if err := m.to.Set(name, v); err != nil {
		return err
	}
	return m.from.Delete(name)
}

func (m moving) Delete(name string) error {
	if err := m.to.Delete(name); err != nil {
		return err
	}
	return m.from.Delete(name)
}

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
