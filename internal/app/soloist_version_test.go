package app

import (
	"testing"
	"time"

	"mygoify/internal/secret"
	"mygoify/internal/soloist"
)

// soloistBuild is a version of Soloist to test what the app says of it.
func soloistBuild() soloist.Version {
	return soloist.Version{Number: "1.3.10.31", Build: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
}

// A build within a fortnight of its expiry is said of once as the app
// starts; one further off is not, nor is one past its expiry, which is
// to be updated rather than waited out.
func TestSoloistExpiryTold(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	v := soloistBuild()
	a.soloistVer = v
	expires := v.Expires()
	a.noteSoloistExpiry(expires.Add(-40 * 24 * time.Hour))
	if a.toast != "" || a.ob.open {
		t.Fatalf("said %q, opened %v, long before its expiry", a.toast, a.ob.open)
	}
	a.noteSoloistExpiry(expires.Add(-3 * 24 * time.Hour))
	if a.toast == "" || a.ob.open {
		t.Fatalf("said %q, opened %v, three days before its expiry", a.toast, a.ob.open)
	}
}

// Soloist exits with 10 once its version expires, and neither launchd nor
// systemd starts it again: the app opens the one step that updates it,
// saying why.
func TestSoloistExpiredAsksToUpdate(t *testing.T) {
	for _, env := range []string{"SPOTIFY_CLIENT_ID", "SPOTIFY_REDIRECT_URI", "SOLOIST_BIN", "SOLOIST_DEVICE_NAME", "SOLOIST_API_KEY", "SOLOIST_DATA_DIR"} {
		t.Setenv(env, "")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Spotify.SoloistBin = "/opt/soloist"
	a.keep = secret.Files(t.TempDir())
	v := soloistBuild()
	a.soloistVer = v
	a.noteSoloistExpiry(v.Expires().Add(time.Minute))
	if !a.ob.open || a.ob.step != stepSoloist {
		t.Fatalf("opened %v at step %d", a.ob.open, a.ob.step)
	}
	if a.ob.binErr == "" {
		t.Error("the step doesn't say why it opened")
	}
}
