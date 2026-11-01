package soloist

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// The test binary stands in for Soloist as CheckKey runs it, with
// FAKE_SOLOIST set: the key "good" logs in; "bad" is refused, and the
// pairing in its data deleted as Soloist deletes it; "garbled" is
// refused as Soloist refuses a key that is no key at all; any other
// never logs in.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_SOLOIST") != "" {
		keyCheckSoloist()
		return
	}
	os.Exit(m.Run())
}

func keyCheckSoloist() {
	fs := flag.NewFlagSet("soloist", flag.ExitOnError)
	key, data := fs.String("k", "", ""), fs.String("D", "", "")
	fs.String("n", "", "")
	fs.String("C", "", "")
	fs.String("w", "", "")
	fs.Parse(os.Args[1:])
	if *key == "bad" {
		fmt.Println("Permanent OAuth error: oauth_invalid_credentials")
		os.RemoveAll(filepath.Join(*data, "settings", "Users"))
	}
	if *key == "garbled" {
		fmt.Println("2026-11-01 20:41:29.939: login failed: make sure --api-key is valid")
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	os.WriteFile(filepath.Join(*data, "ws.addr"), []byte("127.0.0.1"), 0o600)
	os.WriteFile(filepath.Join(*data, "ws.port"), []byte(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)), 0o600)
	http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := context.Background()
		for {
			if _, _, err := c.Read(ctx); err != nil {
				return
			}
			c.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"auth_state","logged_in":%v}`, *key == "good")))
		}
	}))
}

// pairedData is Soloist's data with a pairing, as the Soloist that plays
// keeps it.
func pairedData(t *testing.T) string {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "settings", "Users", "me-user"), 0o700)
	os.WriteFile(filepath.Join(dir, "settings", "Users", "me-user", "prefs"), []byte("session"), 0o600)
	os.WriteFile(filepath.Join(dir, "ws.port"), []byte("1"), 0o600) // the playing Soloist's
	return dir
}

// A key Soloist logs in with checks out; one Spotify refuses is told
// apart, and the pairing it deletes is the copy's, never the real one.
func TestCheckKey(t *testing.T) {
	t.Setenv("FAKE_SOLOIST", "1")
	bin, _ := os.Executable()
	data := pairedData(t)
	if err := CheckKey(context.Background(), bin, "good", data); err != nil {
		t.Errorf("good key: %v", err)
	}
	err := CheckKey(context.Background(), bin, "bad", data)
	if !errors.Is(err, ErrBadKey) {
		t.Errorf("bad key: %v, want ErrBadKey", err)
	}
	if !paired(data) {
		t.Fatal("a bad key deleted the real pairing")
	}
	if b, _ := os.ReadFile(filepath.Join(data, "ws.port")); string(b) != "1" {
		t.Errorf("the playing Soloist's port became %q", b)
	}
}

// A key that never logs in fails once the wait is over, without saying
// the key in its error.
func TestCheckKeyTimesOut(t *testing.T) {
	t.Setenv("FAKE_SOLOIST", "1")
	defer func(d time.Duration) { checkWait = d }(checkWait)
	checkWait = 500 * time.Millisecond
	bin, _ := os.Executable()
	err := CheckKey(context.Background(), bin, "quiet-key", pairedData(t))
	if err == nil || errors.Is(err, ErrBadKey) || strings.Contains(err.Error(), "quiet-key") {
		t.Errorf("err %v, want a timeout that doesn't name the key", err)
	}
}

// A key that is no key at all is told apart at once. Soloist refuses it
// in other words than one Spotify refuses, which CheckKey didn't know:
// it waited out the 30 seconds, the music stopped meanwhile, and the
// listener was told Soloist didn't start rather than that the key was
// wrong.
func TestCheckKeyGarbled(t *testing.T) {
	t.Setenv("FAKE_SOLOIST", "1")
	defer func(d time.Duration) { checkWait = d }(checkWait)
	checkWait = 5 * time.Second
	bin, _ := os.Executable()
	if err := CheckKey(context.Background(), bin, "garbled", pairedData(t)); !errors.Is(err, ErrBadKey) {
		t.Errorf("err %v, want ErrBadKey", err)
	}
}

// Without a pairing there is nothing to try a key on: Soloist isn't run.
func TestCheckKeyNotPaired(t *testing.T) {
	err := CheckKey(context.Background(), "/nonexistent/soloist", "good", t.TempDir())
	if !errors.Is(err, ErrNotPaired) {
		t.Errorf("err %v, want ErrNotPaired", err)
	}
}

// The real Soloist logs in on a copy of its data with the key it plays
// with, and the copy's pairing is all it needs. It runs Soloist, with
// MYGOIFY_SOLOIST_CHECK, SOLOIST_BIN and SOLOIST_API_KEY set; stop the
// service first, as the copy is the same device.
func TestCheckKeyReal(t *testing.T) {
	bin, key := os.Getenv("SOLOIST_BIN"), os.Getenv("SOLOIST_API_KEY")
	if os.Getenv("MYGOIFY_SOLOIST_CHECK") == "" || bin == "" || key == "" {
		t.Skip("set MYGOIFY_SOLOIST_CHECK, SOLOIST_BIN and SOLOIST_API_KEY to try the real Soloist")
	}
	home, _ := os.UserHomeDir()
	bin = strings.Replace(bin, "~", home, 1)
	data := os.Getenv("SOLOIST_DATA_DIR")
	if data == "" {
		data = filepath.Join(home, ".local/share/mygoify/soloist")
	}
	start := time.Now()
	if err := CheckKey(context.Background(), bin, key, data); err != nil {
		t.Fatal(err)
	}
	t.Logf("logged in on the copy in %v", time.Since(start).Round(time.Millisecond))
}

// A new key is tried with the Soloist that plays stopped, and it plays
// on with the new key if Spotify took it, or with the one it had.
func TestChangeKey(t *testing.T) {
	t.Setenv("FAKE_SOLOIST", "1")
	s, ran := fakeService(t)
	s.Bin, _ = os.Executable()
	s.DataDir = pairedData(t)
	s.Key = "good"
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	*ran = nil
	if err := s.ChangeKey(context.Background(), "bad"); !errors.Is(err, ErrBadKey) {
		t.Errorf("bad key: %v, want ErrBadKey", err)
	}
	if s.Key != "good" {
		t.Errorf("a bad key left the key %q", s.Key)
	}
	if !strings.HasPrefix(strings.Join(*ran, ", "), stopCmd()) || !running(*ran) {
		t.Errorf("a bad key ran %s", strings.Join(*ran, ", "))
	}
	*ran = nil
	s.Key = "old"
	if err := s.ChangeKey(context.Background(), "good"); err != nil {
		t.Errorf("good key: %v", err)
	}
	if s.Key != "good" || !running(*ran) {
		t.Errorf("a good key left the key %q, and ran %s", s.Key, strings.Join(*ran, ", "))
	}
}

// stopCmd is what stops the service, as fakeService notes it.
func stopCmd() string {
	if runtime.GOOS == "linux" {
		return "systemctl stop"
	}
	return "launchctl bootout"
}

// running reports whether ran ends with the service started.
func running(ran []string) bool {
	return len(ran) > 0 && (ran[len(ran)-1] == "launchctl kickstart" || ran[len(ran)-1] == "systemctl restart" || ran[len(ran)-1] == "systemctl start")
}
