package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"mygoify/internal/playback"
)

// pausingBackend notes that the app paused it as it quit.
type pausingBackend struct {
	*playback.Fake
	ran *[]string
}

func (b pausingBackend) PauseAndWait(context.Context) error {
	*b.ran = append(*b.ran, "pause")
	return nil
}

// fakeSvc notes how the app starts and stops Soloist's service.
type fakeSvc struct{ ran *[]string }

func (s fakeSvc) Start(atLogin bool) error {
	if atLogin {
		*s.ran = append(*s.ran, "start at login")
	} else {
		*s.ran = append(*s.ran, "start")
	}
	return nil
}

func (s fakeSvc) Stop() error {
	*s.ran = append(*s.ran, "stop")
	return nil
}

func quittingApp(mode string) (*app, *[]string) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Quit = mode
	var ran []string
	a.backend = pausingBackend{playback.NewFake(), &ran}
	a.svc = fakeSvc{&ran}
	return a, &ran
}

// Quitting stops Soloist after pausing it, pauses it alone, or leaves it
// playing, as the settings say; starting again at once, as onboarding
// does, leaves it playing whatever they say.
func TestQuitModes(t *testing.T) {
	for mode, want := range map[string]string{"stop": "pause, stop", "pause": "pause", "play": ""} {
		a, ran := quittingApp(mode)
		a.leaveSoloist()
		if got := strings.Join(*ran, ", "); got != want {
			t.Errorf("quitting with %s ran %q, not %q", mode, got, want)
		}
	}
	a, ran := quittingApp("stop")
	a.relaunching = true
	a.leaveSoloist()
	if len(*ran) > 0 {
		t.Errorf("relaunching ran %v", *ran)
	}
}

// Turning on Soloist at login starts the service so, off the main
// thread.
func TestSoloistAtLogin(t *testing.T) {
	a, ran := quittingApp("stop")
	started := make(chan struct{}, 1)
	a.svc = startNotes{fakeSvc{ran}, started}
	a.settings.Spotify.SoloistAtLogin = true
	a.changeAtLogin()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("not started")
	}
	if got := strings.Join(*ran, ", "); got != "start at login" {
		t.Fatalf("ran %s", got)
	}
}

// startNotes says when the service started, which the app does off the
// main thread.
type startNotes struct {
	fakeSvc
	started chan struct{}
}

func (s startNotes) Start(atLogin bool) error {
	err := s.fakeSvc.Start(atLogin)
	s.started <- struct{}{}
	return err
}

// Closing the window quits the app if the listener says so, rather than
// hiding it.
func TestCloseQuits(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CloseQuits = true
	quits := 0
	defer func(q func()) { quitApp = q }(quitApp)
	quitApp = func() { quits++ }
	a.closing(&mygo.CloseEvent{})
	if quits != 1 {
		t.Fatal("closing the window did not quit")
	}
}
