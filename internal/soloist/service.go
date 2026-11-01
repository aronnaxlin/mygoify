package soloist

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Service runs Soloist as the listener's own service, a launchd agent on
// macOS and a systemd user unit on Linux, so that it plays on once the
// window is closed (DESIGN.md §4.2). The app starts it as it starts, and
// launchd or systemd at login too if AtLogin.
//
// Soloist takes its API key only as an argument (POC.md P9), so the key
// goes into a file only the listener can read, and the service reads it
// from there as it starts: the agent's or unit's file never holds it.
type Service struct {
	Bin        string // Soloist's executable
	Key        string // its API key
	DeviceName string
	DataDir    string // where it keeps its pairing and writes ws.port
	// AtLogin starts it as the listener logs in, so that Spotify Connect
	// lists the computer with the app closed.
	AtLogin bool
	// Dir holds the agent's or unit's file: ~/Library/LaunchAgents or
	// ~/.config/systemd/user, as Start finds it when empty.
	Dir string
	// Logs is where launchd writes what Soloist prints; journald keeps it
	// on Linux.
	Logs string

	// run runs a command of launchctl or systemctl, exec.Command's
	// unless a test swaps it.
	run func(name string, args ...string) error
}

const (
	agentLabel = "io.github.aronnaxlin.mygoify.soloist"
	unitName   = "mygoify-soloist.service"
)

// Start writes the service and starts it, restarting it if what it runs
// with changed, such as the key or the device name, and leaving it be if
// it already runs as it should.
func (s *Service) Start() error {
	if s.run == nil {
		s.run = runCommand
	}
	switch runtime.GOOS {
	case "darwin":
		return s.startAgent()
	case "linux":
		return s.startUnit()
	}
	return fmt.Errorf("soloist: no service on %s", runtime.GOOS)
}

// Stop stops the service, if it runs.
func (s *Service) Stop() error {
	if s.run == nil {
		s.run = runCommand
	}
	switch runtime.GOOS {
	case "darwin":
		target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + agentLabel
		if s.run("launchctl", "print", target) != nil {
			return nil
		}
		return s.run("launchctl", "bootout", target)
	case "linux":
		return s.run("systemctl", "--user", "stop", unitName)
	}
	return fmt.Errorf("soloist: no service on %s", runtime.GOOS)
}

// ChangeKey tries key with CheckKey and starts the service again with it
// if Spotify took it, or with the key it had, returning why not. The
// service is stopped meanwhile: the copy CheckKey tries is the same
// device. Before the first pairing there is nothing to try key on: it
// returns ErrNotPaired, leaving the service be.
func (s *Service) ChangeKey(ctx context.Context, key string) error {
	if !paired(s.DataDir) {
		return ErrNotPaired
	}
	if err := s.Stop(); err != nil {
		return err
	}
	err := CheckKey(ctx, s.Bin, key, s.DataDir)
	if err == nil {
		s.Key = key
	}
	if serr := s.Start(); serr != nil && err == nil {
		err = serr
	}
	return err
}

// keyFile is where the service reads the key from.
func (s *Service) keyFile() string { return filepath.Join(filepath.Dir(s.DataDir), "soloist.key") }

func (s *Service) startAgent() error {
	dir := s.Dir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, "Library/LaunchAgents")
	}
	if err := os.MkdirAll(s.DataDir, 0o700); err != nil {
		return err
	}
	if s.Logs != "" {
		if err := os.MkdirAll(filepath.Dir(s.Logs), 0o755); err != nil {
			return err
		}
	}
	keyChanged, err := writeIfChanged(s.keyFile(), []byte(s.Key), 0o600)
	if err != nil {
		return err
	}
	plist := filepath.Join(dir, agentLabel+".plist")
	old, _ := os.ReadFile(plist)
	agentChanged, err := writeIfChanged(plist, s.agent(s.AtLogin), 0o644)
	if err != nil {
		return err
	}
	// Whether it starts at login, launchd reads from the file only as the
	// listener logs in: the Soloist that plays plays on.
	if agentChanged && bytes.Equal(old, s.agent(!s.AtLogin)) {
		agentChanged = false
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	target := domain + "/" + agentLabel
	loaded := s.run("launchctl", "print", target) == nil
	if loaded && (keyChanged || agentChanged) {
		// Loaded as it was: bootout stops it and forgets the old file.
		if err := s.run("launchctl", "bootout", target); err != nil {
			return err
		}
		loaded = false
	}
	if !loaded {
		if err := s.run("launchctl", "bootstrap", domain, plist); err != nil {
			return err
		}
	}
	// Starts it unless it runs, as RunAtLoad may be off.
	return s.run("launchctl", "kickstart", target)
}

// agent is the launch agent's property list, which starts Soloist at
// login if atLogin. A shell reads the key from its file and hands Soloist
// the rest as its own arguments, so nothing in them needs quoting.
func (s *Service) agent(atLogin bool) []byte {
	args := []string{
		"/bin/sh", "-c", `exec "$0" -n "$1" -k "$(cat "$2")" -D "$3" -w 127.0.0.1:0`,
		s.Bin, s.DeviceName, s.keyFile(), s.DataDir,
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + agentLabel + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range args {
		b.WriteString("\t\t<string>" + xmlEscape(a) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<` + strconv.FormatBool(atLogin) + `/>
	<key>KeepAlive</key>
	<dict>
		<!-- Again after a crash, but not after it exits: it exits with 10
		     once its version expires, and starting it again won't help. -->
		<key>Crashed</key>
		<true/>
	</dict>
	<key>ProcessType</key>
	<string>Interactive</string>
`)
	if s.Logs != "" {
		b.WriteString("\t<key>StandardOutPath</key>\n\t<string>" + xmlEscape(s.Logs) + "</string>\n")
		b.WriteString("\t<key>StandardErrorPath</key>\n\t<string>" + xmlEscape(s.Logs) + "</string>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

func (s *Service) startUnit() error {
	dir := s.Dir
	if dir == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(config, "systemd/user")
	}
	if err := os.MkdirAll(s.DataDir, 0o700); err != nil {
		return err
	}
	keyChanged, err := writeIfChanged(s.keyFile(), []byte("SOLOIST_API_KEY="+envQuote(s.Key)+"\n"), 0o600)
	if err != nil {
		return err
	}
	unitChanged, err := writeIfChanged(filepath.Join(dir, unitName), s.unit(), 0o644)
	if err != nil {
		return err
	}
	if unitChanged {
		if err := s.run("systemctl", "--user", "daemon-reload"); err != nil {
			return err
		}
	}
	// Whether it starts at login, by the link of its [Install] section.
	login := "disable"
	if s.AtLogin {
		login = "enable"
	}
	if err := s.run("systemctl", "--user", login, unitName); err != nil {
		return err
	}
	if keyChanged || unitChanged {
		// Starts it too if it doesn't run.
		return s.run("systemctl", "--user", "restart", unitName)
	}
	return s.run("systemctl", "--user", "start", unitName)
}

// unit is the systemd user unit. ${SOLOIST_API_KEY} comes from its
// EnvironmentFile, and stays one argument within its quotes.
func (s *Service) unit() []byte {
	return []byte(`[Unit]
Description=Spotify Soloist, for MyGoify

[Service]
EnvironmentFile=` + s.keyFile() + `
ExecStart=` + strings.Join([]string{
		systemdQuote(s.Bin), "-n", systemdQuote(s.DeviceName),
		"-k", `"${SOLOIST_API_KEY}"`, "-D", systemdQuote(s.DataDir), "-w", "127.0.0.1:0",
	}, " ") + `
# Again after a crash, but not after it exits: it exits with 10 once its
# version expires, and starting it again won't help.
Restart=on-abnormal

[Install]
WantedBy=default.target
`)
}

// writeIfChanged writes data to path unless it holds it already, saying
// whether it wrote.
func writeIfChanged(path string, data []byte, perm os.FileMode) (bool, error) {
	old, err := os.ReadFile(path)
	if err == nil && bytes.Equal(old, data) {
		return false, os.Chmod(path, perm)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return false, err
	}
	// WriteFile keeps the mode of a file that was there.
	return true, os.Chmod(path, perm)
}

func runCommand(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("soloist: %s %s: %w: %s", name, strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return nil
}

func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// systemdQuote quotes s as one word of a unit's command line, where $
// and % would otherwise expand.
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `$$`, `%`, `%%`)
	return `"` + r.Replace(s) + `"`
}

// envQuote quotes s as an EnvironmentFile's value, which expands nothing.
func envQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
