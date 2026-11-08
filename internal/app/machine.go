package app

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// defaultDeviceName is how Spotify Connect lists Soloist unless the
// listener names it: by the app and the computer, as two of the
// listener's computers each running it showed as two "MyGoify", and
// Spotify moved the music to the one the listener didn't mean.
func defaultDeviceName() string {
	return deviceNameOf(machineName())
}

func deviceNameOf(machine string) string {
	if machine == "" {
		return "MyGoify"
	}
	return "MyGoify (" + machine + ")"
}

// machineName is the computer's name as people know it: the Computer
// Name of macOS's settings, Linux's pretty hostname, else its hostname
// without its domain. It asks the system once.
var machineName = sync.OnceValue(func() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("scutil", "--get", "ComputerName").Output(); err == nil {
			if name := strings.TrimSpace(string(out)); name != "" {
				return name
			}
		}
	}
	if b, err := os.ReadFile("/etc/machine-info"); err == nil {
		if name := prettyHostname(string(b)); name != "" {
			return name
		}
	}
	host, _ := os.Hostname()
	return shortHost(host)
})

// prettyHostname reads PRETTY_HOSTNAME of /etc/machine-info, whose values
// may be quoted as a shell's are.
func prettyHostname(info string) string {
	for line := range strings.Lines(info) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "PRETTY_HOSTNAME="); ok {
			if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
				v = v[1 : len(v)-1]
			}
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// shortHost is a hostname without its domain: Aronnaxs-MacBook-Air.local
// is Aronnaxs-MacBook-Air.
func shortHost(host string) string {
	host, _, _ = strings.Cut(strings.TrimSpace(host), ".")
	return host
}
