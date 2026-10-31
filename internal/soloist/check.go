package soloist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// ErrNotPaired is what CheckKey returns when Soloist's data holds no
// pairing to try the key on, as before the first: the key shows itself
// wrong only as pairing fails.
var ErrNotPaired = errors.New("soloist: not paired yet")

// ErrBadKey is what CheckKey returns when Spotify refused the key.
var ErrBadKey = errors.New("soloist: Spotify refused the API key")

// checkWait is how long Soloist has to log in with a key tried.
var checkWait = 30 * time.Second

// CheckKey tries key with Soloist's executable bin on a copy of dataDir,
// and returns nil once Soloist logs in with it, within 30 seconds. A
// wrong key makes Soloist delete the pairing in its data (DESIGN.md
// §4.2): a key not checked so never meets the real one.
func CheckKey(ctx context.Context, bin, key, dataDir string) error {
	if !paired(dataDir) {
		return ErrNotPaired
	}
	tmp, err := os.MkdirTemp("", "mygoify-key-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	data := filepath.Join(tmp, "data")
	if err := copyPairing(dataDir, data); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, checkWait)
	defer cancel()
	out := &refusal{}
	// Its own name, and no device ID copied: the Soloist that plays stays
	// the device it is. Its own cache, as two may not share one.
	cmd := exec.CommandContext(ctx, bin, "-n", "MyGoify key check", "-k", key, "-D", data, "-C", filepath.Join(tmp, "cache"), "-w", "127.0.0.1:0")
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("soloist: can't start: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cancel(); <-done }()

	logged := make(chan error, 1)
	go func() { logged <- waitLoggedIn(ctx, data) }()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-logged:
			if err == nil {
				return nil
			}
			if out.seen() {
				return ErrBadKey
			}
			if ctx.Err() != nil {
				return errors.New("soloist: didn't log in within 30 seconds")
			}
			return err
		case <-done:
			done <- nil // for the deferred wait
			if out.seen() {
				return ErrBadKey
			}
			return errors.New("soloist: quit before logging in")
		case <-tick.C:
			if out.seen() {
				return ErrBadKey
			}
		}
	}
}

// paired reports whether Soloist's data holds a pairing: a listener's
// directory in its settings.
func paired(dataDir string) bool {
	users, _ := os.ReadDir(filepath.Join(dataDir, "settings", "Users"))
	for _, u := range users {
		if u.IsDir() {
			return true
		}
	}
	return false
}

// copyPairing copies Soloist's data but what is the running Soloist's
// own: where it listens, its lock and process, its device and caches.
func copyPairing(from, to string) error {
	skip := map[string]bool{"ws.addr": true, "ws.port": true, "soloist.pid": true, ".lock": true, ".device_id": true, "cache": true, "crashpad": true}
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if skip[rel] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o600)
	})
}

// waitLoggedIn connects to the Soloist of dataDir once it listens, and
// returns once it says it is logged in.
func waitLoggedIn(ctx context.Context, dataDir string) error {
	b := &Backend{cfg: Config{DataDir: dataDir}}
	var addr string
	for {
		var err error
		if addr, err = b.addr(); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	var conn *websocket.Conn
	for {
		var err error
		if conn, _, err = websocket.Dial(ctx, addr, nil); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	defer conn.CloseNow()
	conn.SetReadLimit(16 << 20)
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"command":"get_auth_state"}`)); err != nil {
		return err
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var ev event
		if json.Unmarshal(data, &ev) == nil && ev.Type == "auth_state" && ev.LoggedIn != nil && *ev.LoggedIn {
			return nil
		}
	}
}

// refusal reads what Soloist prints for Spotify refusing its key; it
// keeps none of it, as some may be the key's.
type refusal struct {
	mu   sync.Mutex
	line []byte
	hit  bool
}

var _ io.Writer = (*refusal)(nil)

func (r *refusal) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.line = append(r.line, p...)
	for {
		i := bytes.IndexByte(r.line, '\n')
		if i < 0 {
			break
		}
		if s := string(r.line[:i]); strings.Contains(s, "oauth_invalid_credentials") || strings.Contains(s, "Access denied") {
			r.hit = true
		}
		r.line = r.line[i+1:]
	}
	if len(r.line) > 4096 {
		r.line = r.line[len(r.line)-4096:]
	}
	return len(p), nil
}

func (r *refusal) seen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hit
}
