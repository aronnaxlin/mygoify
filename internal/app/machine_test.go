package app

import "testing"

// Soloist is named by the computer, as macOS and Linux name it to people:
// a pretty hostname, quoted or not, before the hostname, which loses its
// domain; without a name, the app's alone.
func TestDeviceName(t *testing.T) {
	for _, c := range []struct{ info, host, want string }{
		{"PRETTY_HOSTNAME=\"Aronnax's ThinkPad\"\nCHASSIS=laptop\n", "omarchy", "MyGoify (Aronnax's ThinkPad)"},
		{"CHASSIS=laptop\nPRETTY_HOSTNAME=书房\n", "omarchy", "MyGoify (书房)"},
		{"", "Aronnaxs-MacBook-Air.local", "MyGoify (Aronnaxs-MacBook-Air)"},
		{"PRETTY_HOSTNAME=\n", "", "MyGoify"},
	} {
		name := prettyHostname(c.info)
		if name == "" {
			name = shortHost(c.host)
		}
		if got := deviceNameOf(name); got != c.want {
			t.Errorf("machine-info %q, host %q: %q, want %q", c.info, c.host, got, c.want)
		}
	}
}
