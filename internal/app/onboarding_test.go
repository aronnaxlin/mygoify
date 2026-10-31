package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/auth"
	"mygoify/internal/secret"
	"mygoify/internal/soloist"
)

// onboarding is an app at the onboarding dialog, with what it reaches
// outside replaced, and the answers the app posts held until the test
// runs them.
type onboarding struct {
	t      *testing.T
	a      *app
	tt     *ui.Tester
	keep   secret.Store
	posted chan func()
	bin    string // an executable to choose

	opened     []string
	loggedIn   []auth.Config
	loginErr   error
	keyErr     error
	keysTried  []string
	started    chan string // the keys Soloist's service was started with
	paired     func()      // what Soloist's watch calls as it is logged in
	relaunched int
	fetched    []string // what was downloaded
	archive    []byte   // what the download brings; fetchHeld holds it up
	fetchHeld  bool
	home       string // where Soloist is unpacked
}

func newOnboarding(t *testing.T) *onboarding {
	for _, env := range []string{"SPOTIFY_CLIENT_ID", "SPOTIFY_REDIRECT_URI", "SOLOIST_BIN", "SOLOIST_DEVICE_NAME", "SOLOIST_API_KEY", "SOLOIST_DATA_DIR"} {
		t.Setenv(env, "")
	}
	o := &onboarding{t: t, keep: secret.Files(t.TempDir()), posted: make(chan func(), 8), started: make(chan string, 8)}
	o.bin = filepath.Join(t.TempDir(), "soloist")
	if err := os.WriteFile(o.bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	o.home = t.TempDir()
	a := newTestApp()
	a.settings = defaultSettings()
	a.keep = o.keep
	a.inline = func(f func()) { o.posted <- f }
	o.a = a

	real := struct {
		open     func(string) error
		choose   func() string
		login    func(context.Context, auth.Config, secret.Store) error
		check    func(context.Context, string, string, string) error
		start    func(spotifySettings, string) error
		watch    func(spotifySettings, func())
		relaunch func()
		fetch    func(context.Context, string) (io.ReadCloser, int64, error)
		runs     func(context.Context, string) error
		home     func() string
	}{openURL, chooseSoloist, spotifyLogin, checkKey, startSoloist, watchSoloist, relaunch, soloistFetch, soloistRuns, soloistHome}
	t.Cleanup(func() {
		openURL, chooseSoloist, spotifyLogin, checkKey, startSoloist, watchSoloist, relaunch =
			real.open, real.choose, real.login, real.check, real.start, real.watch, real.relaunch
		soloistFetch, soloistRuns, soloistHome = real.fetch, real.runs, real.home
	})
	soloistFetch = func(ctx context.Context, url string) (io.ReadCloser, int64, error) {
		o.fetched = append(o.fetched, url)
		if o.fetchHeld {
			<-ctx.Done()
			return nil, 0, ctx.Err()
		}
		return io.NopCloser(bytes.NewReader(o.archive)), int64(len(o.archive)), nil
	}
	soloistRuns = func(context.Context, string) error { return nil }
	soloistHome = func() string { return o.home }
	openURL = func(u string) error { o.opened = append(o.opened, u); return nil }
	chooseSoloist = func() string { return o.bin }
	spotifyLogin = func(ctx context.Context, cfg auth.Config, keep secret.Store) error {
		o.loggedIn = append(o.loggedIn, cfg)
		if o.loginErr != nil {
			return o.loginErr
		}
		return keep.Set(secret.SpotifyLogin, []byte(`{"refresh_token":"r"}`))
	}
	checkKey = func(ctx context.Context, bin, key, dir string) error {
		o.keysTried = append(o.keysTried, key)
		return o.keyErr
	}
	startSoloist = func(cfg spotifySettings, key string) error { o.started <- key; return nil }
	watchSoloist = func(cfg spotifySettings, paired func()) { o.paired = paired }
	relaunch = func() { o.relaunched++ }
	return o
}

// open shows the dialog at the first step not done.
func (o *onboarding) open() {
	o.a.openOnboarding()
	o.tt = ui.NewTester(o.a.view, 1200, 800)
	o.tt.Advance(time.Second)
	o.tt.Frame()
}

func (o *onboarding) click(label string) {
	o.t.Helper()
	if err := o.tt.Click(label); err != nil {
		o.t.Fatalf("%v; the dialog shows %q", err, o.tt.Texts())
	}
	o.tt.Frame()
}

// typeInto types into the field named label.
func (o *onboarding) typeInto(label, s string) {
	o.t.Helper()
	o.click(label)
	o.tt.Type(s)
	o.tt.Frame()
}

// settle runs the one answer the app posted, as its window would.
func (o *onboarding) settle() {
	o.t.Helper()
	select {
	case f := <-o.posted:
		f()
	case <-time.After(2 * time.Second):
		o.t.Fatal("the app posted nothing")
	}
	if o.tt != nil {
		o.tt.Frame()
	}
}

// noPost fails if the app posted an answer.
func (o *onboarding) noPost() {
	o.t.Helper()
	select {
	case <-o.posted:
		o.t.Fatal("the app posted an answer")
	case <-time.After(50 * time.Millisecond):
	}
}

func (o *onboarding) kept(name string) (string, bool) {
	b, err := o.keep.Get(name)
	return string(b), err == nil
}

// set makes the first steps done, up to but not with step.
func (o *onboarding) set(upto int) {
	o.t.Helper()
	s := &o.a.settings.Spotify
	if upto > stepApp {
		s.ClientID, s.RedirectURI = "0123456789abcdef0123456789abcdef", defaultRedirect
	}
	if upto > stepLogin {
		o.keep.Set(secret.SpotifyLogin, []byte(`{"refresh_token":"r"}`))
	}
	if upto > stepSoloist {
		s.SoloistBin = o.bin
	}
	if upto > stepKey {
		o.keep.Set(secret.SoloistKey, []byte("the-key"))
	}
}

// Once a Client ID is filled in, the next step is open, with the callback
// address kept as it was typed; one that isn't of 127.0.0.1 is no answer.
func TestOnboardingClientIDGoesOn(t *testing.T) {
	o := newOnboarding(t)
	o.open()
	if o.a.ob.step != stepApp {
		t.Fatalf("opens at step %d", o.a.ob.step)
	}
	o.typeInto("填写 Client ID", "0123456789abcdef0123456789abcdef")
	o.a.ob.redirect = "https://example.com/callback"
	o.tt.Frame()
	o.click("保存")
	if _, ok := o.a.settings.Spotify, o.a.ob.appErr; ok == "" || o.a.settings.Spotify.ClientID != "" {
		t.Fatalf("kept %+v with the redirect off 127.0.0.1", o.a.settings.Spotify)
	}
	o.a.ob.redirect = "http://127.0.0.1:50000/cb"
	o.tt.Frame()
	o.click("保存")
	got := o.a.settings.Spotify
	if got.ClientID != "0123456789abcdef0123456789abcdef" || got.RedirectURI != "http://127.0.0.1:50000/cb" {
		t.Fatalf("kept %+v", got)
	}
	if o.a.ob.step != stepLogin {
		t.Errorf("goes on to step %d, want the login's", o.a.ob.step)
	}
}

// A login is of the app it was made to: a new Client ID forgets it, and
// the same one, saved again, doesn't.
func TestOnboardingNewClientIDForgetsLogin(t *testing.T) {
	o := newOnboarding(t)
	o.set(stepSoloist)
	o.a.look()
	o.a.ob.clientID = o.a.settings.Spotify.ClientID
	o.a.saveApp()
	if _, ok := o.kept(secret.SpotifyLogin); !ok {
		t.Fatal("the same Client ID forgot the login")
	}
	o.a.ob.clientID = "fedcba9876543210fedcba9876543210"
	o.a.saveApp()
	if _, ok := o.kept(secret.SpotifyLogin); ok {
		t.Error("another Client ID kept the login")
	}
}

// The login is made with the Client ID and the callback address kept, in
// the secret store, and goes on once it is; a refused one keeps nothing.
func TestOnboardingLogin(t *testing.T) {
	o := newOnboarding(t)
	o.set(stepLogin)
	o.open()
	o.loginErr = auth.ErrDenied
	o.click("在浏览器中登录")
	o.settle()
	if _, ok := o.kept(secret.SpotifyLogin); ok || o.a.ob.step != stepLogin || o.a.ob.loginErr == "" {
		t.Fatalf("a refused login kept one, or went on (step %d, %q)", o.a.ob.step, o.a.ob.loginErr)
	}
	o.loginErr = nil
	o.click("在浏览器中登录")
	o.settle()
	if _, ok := o.kept(secret.SpotifyLogin); !ok || o.a.ob.step != stepSoloist {
		t.Fatalf("login kept: %v, at step %d", ok, o.a.ob.step)
	}
	want := auth.Config{ClientID: "0123456789abcdef0123456789abcdef", RedirectURI: defaultRedirect}
	if len(o.loggedIn) != 2 || o.loggedIn[1] != want {
		t.Errorf("logged in with %+v, want %+v", o.loggedIn, want)
	}
}

// Closing the dialog while the browser is waited for drops the login: its
// answer, as it comes, changes nothing.
func TestOnboardingClosedLoginIsDropped(t *testing.T) {
	o := newOnboarding(t)
	o.set(stepLogin)
	o.open()
	o.click("在浏览器中登录")
	o.a.closeOnboarding()
	o.settle()
	if o.a.ob.have.login || o.a.ob.step != stepLogin || o.a.ob.dirty {
		t.Errorf("a login closed went on: %+v", o.a.ob)
	}
}

// Soloist's executable is kept once it is one; a file that can't run is
// not.
func TestOnboardingChoosesSoloist(t *testing.T) {
	o := newOnboarding(t)
	o.set(stepSoloist)
	o.open()
	plain := filepath.Join(t.TempDir(), "soloist.tar.gz")
	os.WriteFile(plain, []byte("x"), 0o644)
	o.bin, plain = plain, o.bin
	o.click("选择已下载的文件…")
	o.settle() // an archive that is none
	if o.a.settings.Spotify.SoloistBin != "" || o.a.ob.binErr == "" {
		t.Fatal("kept a file that can't run")
	}
	o.bin = plain
	o.click("选择已下载的文件…")
	if o.a.settings.Spotify.SoloistBin != o.bin || o.a.ob.step != stepKey {
		t.Errorf("kept %q, at step %d", o.a.settings.Spotify.SoloistBin, o.a.ob.step)
	}
}

// A key is kept when it is good, or when Soloist has no pairing to try it
// on; one that is not is not, and neither it nor what is said of it shows.
func TestOnboardingKey(t *testing.T) {
	for _, c := range []struct {
		name  string
		err   error
		kept  bool
		state keyState
	}{
		{"valid", nil, true, keyValid},
		{"not paired", soloist.ErrNotPaired, true, keyUnverified},
		{"refused", fmt.Errorf("%w: the-new-key", soloist.ErrBadKey), false, keyInvalid},
		{"soloist failed", errors.New("exec: the-new-key"), false, keyFailed},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := newOnboarding(t)
			o.set(stepKey)
			o.open()
			o.keyErr = c.err
			o.typeInto("填写 API key", "the-new-key")
			for _, s := range o.tt.Texts() {
				if strings.Contains(s, "the-new-key") {
					t.Fatalf("the key shows: %q", s)
				}
			}
			o.click("验证")
			o.settle()
			key, ok := o.kept(secret.SoloistKey)
			if ok != c.kept || (ok && key != "the-new-key") {
				t.Errorf("kept %q (%v), want kept: %v", key, ok, c.kept)
			}
			if o.a.ob.keyState != c.state {
				t.Errorf("key is %d, want %d", o.a.ob.keyState, c.state)
			}
			for _, s := range o.tt.Texts() {
				if strings.Contains(s, "the-new-key") {
					t.Errorf("the key shows: %q", s)
				}
			}
			if c.kept {
				if o.a.ob.step != stepPair || o.a.ob.key != "" {
					t.Errorf("at step %d, the field holds %q", o.a.ob.step, o.a.ob.key)
				}
			}
		})
	}
}

// The trial of a key, which takes up to 30 seconds, is given up on at the
// listener's word: it is told to stop, and what it answers is dropped.
func TestOnboardingKeyTrialCanBeCancelled(t *testing.T) {
	o := newOnboarding(t)
	o.set(stepKey)
	o.open()
	stopped := make(chan struct{})
	checkKey = func(ctx context.Context, bin, key, dir string) error {
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}
	o.typeInto("填写 API key", "the-new-key")
	o.click("验证")
	if o.a.ob.keyState != keyChecking {
		t.Fatal("not trying the key")
	}
	o.click("取消")
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the trial was not told to stop")
	}
	o.settle()
	if _, ok := o.kept(secret.SoloistKey); ok || o.a.ob.keyState != keyIdle {
		t.Errorf("a cancelled trial kept the key or said %d", o.a.ob.keyState)
	}
}

// A key that was good and is replaced by a wrong one is still the key
// kept, which Soloist runs with.
func TestOnboardingWrongKeyKeepsTheOld(t *testing.T) {
	o := newOnboarding(t)
	o.set(stepPair)
	o.a.look()
	o.a.ob.step = stepKey
	o.keyErr = errors.New("no")
	o.a.ob.key = "another"
	o.a.submitKey()
	o.settle()
	if key, _ := o.kept(secret.SoloistKey); key != "the-key" {
		t.Errorf("the key kept is %q", key)
	}
	select {
	case k := <-o.started:
		t.Errorf("Soloist started with %q after a wrong key", k)
	default:
	}
}

// The last step waits for Soloist to log in, starting it with the key
// kept, and then offers to start the app again.
func TestOnboardingPairing(t *testing.T) {
	o := newOnboarding(t)
	o.set(stepPair)
	o.open()
	if o.a.ob.step != stepPair {
		t.Fatalf("opens at step %d", o.a.ob.step)
	}
	if o.paired == nil {
		t.Fatal("Soloist is not watched")
	}
	select {
	case k := <-o.started:
		if k != "the-key" {
			t.Errorf("Soloist started with %q", k)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Soloist was not started")
	}
	if o.a.complete() {
		t.Fatal("complete before Soloist logs in")
	}
	o.paired()
	o.settle()
	if !o.a.ob.paired || !o.a.complete() {
		t.Fatal("not done once Soloist logged in")
	}
	o.click("重新启动")
	if o.relaunched != 1 {
		t.Errorf("started again %d times", o.relaunched)
	}
}

// Doing a step again leaves what the other steps set up alone: a new key
// replaces the key, and nothing else.
func TestOnboardingRedoesOneStep(t *testing.T) {
	o := newOnboarding(t)
	o.set(steps)
	o.a.ob.paired, o.a.live = true, true
	o.open()
	if o.a.ob.step != -1 && o.a.ob.step != stepPair {
		t.Fatalf("opens at step %d with everything done", o.a.ob.step)
	}
	before := o.a.settings.Spotify
	login, _ := o.kept(secret.SpotifyLogin)
	o.click("修改API key")
	if o.a.ob.step != stepKey {
		t.Fatalf("at step %d", o.a.ob.step)
	}
	o.typeInto("填写 API key", "the-next-key")
	o.click("验证")
	o.settle()
	if key, _ := o.kept(secret.SoloistKey); key != "the-next-key" {
		t.Errorf("key %q", key)
	}
	if now, _ := o.kept(secret.SpotifyLogin); now != login || o.a.settings.Spotify != before {
		t.Errorf("the key's step changed the login or the settings: %q, %+v", now, o.a.settings.Spotify)
	}
}

// Signing out forgets the login and nothing else.
func TestSignOutForgetsLogin(t *testing.T) {
	o := newOnboarding(t)
	o.set(steps)
	before := o.a.settings.Spotify
	o.a.signOut()
	if _, ok := o.kept(secret.SpotifyLogin); ok {
		t.Error("the login is kept")
	}
	if key, ok := o.kept(secret.SoloistKey); !ok || key != "the-key" || o.a.settings.Spotify != before {
		t.Errorf("signing out changed the key (%q) or the settings (%+v)", key, o.a.settings.Spotify)
	}
	if o.a.profile != nil {
		t.Error("still signed in")
	}
}

// Without a login, the first launch opens onboarding; once the listener
// puts it off, it stays closed.
func TestOnboardingDismissed(t *testing.T) {
	o := newOnboarding(t)
	o.open()
	o.click("稍后再说")
	if o.a.ob.open || !o.a.settings.OnboardingDismissed {
		t.Errorf("open %v, dismissed %v", o.a.ob.open, o.a.settings.OnboardingDismissed)
	}
}

// With NM_FRAMES set, saves each step of onboarding in the light and the
// dark.
func TestOnboardingFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	scenes := []struct {
		name string
		set  func(o *onboarding)
	}{
		{"1-app", func(o *onboarding) {}},
		{"2-login", func(o *onboarding) { o.set(stepLogin) }},
		{"2-login-waiting", func(o *onboarding) { o.set(stepLogin); o.a.ob.loginWaiting = true }},
		{"3-soloist", func(o *onboarding) { o.set(stepSoloist) }},
		{"3-soloist-confirm", func(o *onboarding) { o.set(stepSoloist); o.a.ob.dl.confirm = true }},
		{"3-soloist-progress", func(o *onboarding) {
			o.set(stepSoloist)
			o.a.ob.dl = download{busy: true, done: 13_400_000, total: 31_000_000}
		}},
		{"4-key", func(o *onboarding) { o.set(stepKey) }},
		{"4-key-checking", func(o *onboarding) { o.set(stepKey); o.a.ob.key, o.a.ob.keyState = "abcdefabcdef", keyChecking }},
		{"4-key-invalid", func(o *onboarding) { o.set(stepKey); o.a.ob.key, o.a.ob.keyState = "abcdefabcdef", keyInvalid }},
		{"5-pair", func(o *onboarding) { o.set(stepPair) }},
		{"6-done", func(o *onboarding) { o.set(steps); o.a.ob.paired, o.a.ob.dirty = true, true }},
	}
	for _, s := range scenes {
		for _, dark := range []bool{false, true} {
			o := newOnboarding(t)
			s.set(o)
			o.a.look()
			o.a.ob.open = true
			if s.name == "6-done" {
				o.a.ob.step = -1
			} else {
				o.a.ob.step = o.a.firstUndone()
			}
			o.tt = ui.NewTester(o.a.view, 1200, 800)
			if s.name == "1-app" && !dark {
				// The shortest window the app allows.
				o.tt.SetSize(680, 580)
			}
			o.tt.SetDark(dark)
			o.tt.Advance(450 * time.Millisecond)
			o.tt.Frame()
			name := s.name
			if dark {
				name += "-dark"
			}
			f, _ := os.Create(filepath.Join(dir, "onboarding-"+name+".png"))
			png.Encode(f, o.tt.Image())
			f.Close()
		}
	}
}
