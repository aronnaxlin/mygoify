package soloist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeService is a Service whose launchctl or systemctl only notes what
// it was asked, and whose agent is loaded once bootstrapped.
func fakeService(t *testing.T) (*Service, *[]string) {
	root := t.TempDir()
	var ran []string
	loaded := false
	s := &Service{
		Bin:        "/opt/soloist",
		Key:        "key_1",
		DeviceName: "MyGoify",
		DataDir:    filepath.Join(root, "data/soloist"),
		Dir:        filepath.Join(root, "services"),
	}
	s.run = func(name string, args ...string) error {
		cmd := name + " " + args[0]
		if name == "systemctl" {
			cmd = name + " " + args[1]
		}
		switch cmd {
		case "launchctl print":
			if !loaded {
				return os.ErrNotExist
			}
			return nil
		case "launchctl bootstrap":
			loaded = true
		case "launchctl bootout":
			loaded = false
		}
		ran = append(ran, cmd)
		return nil
	}
	return s, &ran
}

// Opening the app again leaves the Soloist that plays be, and a new key
// restarts it, on macOS.
func TestAgentRestartsOnlyWhenChanged(t *testing.T) {
	s, ran := fakeService(t)
	if err := s.startAgent(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*ran, ", "); got != "launchctl bootstrap, launchctl kickstart" {
		t.Fatalf("first start ran %s", got)
	}
	*ran = nil
	if err := s.startAgent(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*ran, ", "); got != "launchctl kickstart" {
		t.Fatalf("starting again ran %s, stopping what plays", got)
	}
	*ran = nil
	s.Key = "key_2"
	if err := s.startAgent(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*ran, ", "); got != "launchctl bootout, launchctl bootstrap, launchctl kickstart" {
		t.Fatalf("a new key ran %s", got)
	}
}

// Opening the app again leaves the Soloist that plays be, and a new
// device name restarts it, on Linux.
func TestUnitRestartsOnlyWhenChanged(t *testing.T) {
	s, ran := fakeService(t)
	if err := s.startUnit(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*ran, ", "); got != "systemctl daemon-reload, systemctl restart" {
		t.Fatalf("first start ran %s", got)
	}
	*ran = nil
	if err := s.startUnit(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*ran, ", "); got != "systemctl start" {
		t.Fatalf("starting again ran %s, stopping what plays", got)
	}
	*ran = nil
	s.DeviceName = "Desk"
	if err := s.startUnit(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*ran, ", "); got != "systemctl daemon-reload, systemctl restart" {
		t.Fatalf("a new name ran %s", got)
	}
}

// The key stays out of the agent's and the unit's files, which anyone
// may read, and in a file only the listener can.
func TestKeyOnlyInPrivateFile(t *testing.T) {
	s, _ := fakeService(t)
	s.Key = "secret_key"
	if err := s.startAgent(); err != nil {
		t.Fatal(err)
	}
	if err := s.startUnit(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{agentLabel + ".plist", unitName} {
		b, err := os.ReadFile(filepath.Join(s.Dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "secret_key") {
			t.Errorf("%s holds the key", f)
		}
	}
	fi, err := os.Stat(s.keyFile())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("key file is %v", fi.Mode().Perm())
	}
}

// A device name with spaces, quotes or $ stays one argument of the unit's
// command line, unexpanded.
func TestUnitQuotesDeviceName(t *testing.T) {
	s, _ := fakeService(t)
	s.DeviceName = `Lin's "Desk" $HOME 100%`
	got := string(s.unit())
	want := `-n "Lin's \"Desk\" $$HOME 100%%"`
	if !strings.Contains(got, want) {
		t.Errorf("unit has no %s:\n%s", want, got)
	}
}
