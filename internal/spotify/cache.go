package spotify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// The library keeps what it reads in its Cache directory, if set, for the
// next time the app starts: the Web API allows the developer account few
// calls (DESIGN.md §5.3), and starting the app asked for everything again.

// kept is what a cache file holds: a value and when it was read.
type kept[T any] struct {
	At time.Time
	V  T
}

func (l *Library) cacheFile(name string) string {
	if l.Cache == "" {
		return ""
	}
	return filepath.Join(l.Cache, name+".json")
}

// load returns what name keeps, and when it was read.
func load[T any](l *Library, name string) (v T, at time.Time, ok bool) {
	f := l.cacheFile(name)
	if f == "" {
		return v, at, false
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return v, at, false
	}
	var k kept[T]
	if json.Unmarshal(b, &k) != nil {
		return v, at, false
	}
	return k.V, k.At, true
}

// store keeps v as name, read now.
func (l *Library) store(name string, v any) {
	f := l.cacheFile(name)
	if f == "" {
		return
	}
	b, err := json.Marshal(kept[any]{time.Now(), v})
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(f), 0o700)
	// Written whole, then renamed: a crash leaves the old file.
	tmp := f + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, f)
	}
}

// forget drops what name keeps: it changed.
func (l *Library) forget(name string) {
	if f := l.cacheFile(name); f != "" {
		os.Remove(f)
	}
}

// cached returns what name keeps if read within fresh, or reads it anew
// with read and keeps that. When reading fails, as when Spotify limits
// the app, what name keeps answers instead, however old.
func cached[T any](l *Library, name string, fresh time.Duration, read func() (T, error)) (T, error) {
	v, at, ok := load[T](l, name)
	if ok && time.Since(at) < fresh {
		return v, nil
	}
	nv, err := read()
	if err != nil {
		if ok {
			return v, nil
		}
		return nv, err
	}
	l.store(name, nv)
	return nv, nil
}

// mine names a file of the listener's: another who signs in on the same
// computer has their own.
func (l *Library) mine(name string) (string, error) {
	if l.Cache == "" {
		return name, nil // kept nowhere: who listens doesn't matter
	}
	me, err := l.Me()
	if err != nil {
		return "", err
	}
	return me.ID + "/" + name, nil
}
