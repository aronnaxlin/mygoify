package app

import (
	"context"
	"errors"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"mygoify/internal/auth"
	"mygoify/internal/playback"
	"mygoify/internal/secret"
	"mygoify/internal/soloist"
)

// Onboarding (DESIGN.md §4.2): five steps, each of which can be done again
// alone, as what it set up stops working. These reach outside the app;
// tests replace them.
var (
	openURL = func(u string) error { return mygo.Shell.OpenExternal(u) }
	// chooseSoloist asks for Soloist's executable, or returns "".
	chooseSoloist = func() string {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "选择 Soloist", ButtonLabel: "选择"})
		if err != nil || len(paths) == 0 {
			return ""
		}
		return paths[0]
	}
	// spotifyLogin has the listener log in in the browser, and keeps the
	// login in keep.
	spotifyLogin = func(ctx context.Context, cfg auth.Config, keep secret.Store) error {
		sess := auth.NewSession(cfg, secret.Entry{Store: keep, Name: secret.SpotifyLogin})
		return sess.Login(ctx, openURL)
	}
	// checkKey tries a key on a copy of Soloist's data (DESIGN.md §4.2):
	// until it holds, the key never meets the pairing.
	checkKey = soloist.CheckKey
	// startSoloist starts Soloist's service with the key, or restarts it
	// as the key or its executable changed.
	startSoloist = func(cfg spotifySettings, key string) error {
		bin := expandHome(cfg.SoloistBin)
		svc := &soloist.Service{Bin: bin, Key: key, DeviceName: cfg.DeviceName, DataDir: soloistDataDir(), Logs: filepath.Join(dataDir(), "soloist.log")}
		return svc.Start()
	}
	// watchSoloist calls paired each time Soloist says it is logged in to
	// Spotify: as it first is, and again as it restarts.
	watchSoloist = func(cfg spotifySettings, paired func()) {
		soloist.New(soloist.Config{DataDir: soloistDataDir(), DeviceName: cfg.DeviceName, LoggedIn: paired}).Watch(func(playback.State) {})
	}
	relaunch = func() { mygo.App.Relaunch() }
)

// Where the listener makes the app, and gets Soloist.
const (
	developerDashboardURL = "https://developer.spotify.com/dashboard"
	// defaultRedirect is what the listener registers as the app's Redirect
	// URI, unless they have a reason to use another port.
	defaultRedirect = "http://127.0.0.1:45231/callback"
)

// The steps, in the order they are done.
const (
	stepApp = iota
	stepLogin
	stepSoloist
	stepKey
	stepPair
	steps
)

// keyState is how trying the API key is going.
type keyState int

const (
	keyIdle keyState = iota
	keyChecking
	keyInvalid // Spotify refused it
	keyFailed  // Soloist didn't start, or didn't log in: it may be tried again
	keyValid
	keyUnverified // kept, for Soloist has no pairing to try it on yet
)

// onboardState is the onboarding dialog.
type onboardState struct {
	open  bool
	shown bool // the dialog was open as it was last built
	step  int  // the step that shows its controls
	// have is what each step has set up, as it was last looked at.
	have struct{ app, login, bin, key bool }
	paired,
	watching bool // Soloist is watched, for it to say it is logged in
	// dirty says something changed that a running app doesn't take up
	// until it starts again.
	dirty bool

	clientID, redirect string // the fields of the first step
	appErr             string
	loginWaiting       bool
	loginErr           string
	loginGen           int // counts logins, to drop the answers of those dropped
	stopLogin          context.CancelFunc
	binErr             string
	dl                 download
	key                string // what the field holds, never shown
	keyState           keyState
	keyGen             int
	stopKey            context.CancelFunc
}

// download is Soloist being downloaded, or unpacked from a file chosen.
type download struct {
	confirm     bool // the listener is asked where it comes from
	busy        bool
	done, total int64 // bytes; total is -1 if not known
	gen         int   // counts downloads, to drop the answers of those dropped
	stop        context.CancelFunc
}

// vault is the store of the listener's secrets.
func (a *app) vault() secret.Store {
	if a.keep != nil {
		return a.keep
	}
	return secrets()
}

// soloistDataDir is where Soloist keeps its pairing, as connectSpotify has
// it.
func soloistDataDir() string {
	if dir := os.Getenv("SOLOIST_DATA_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(dataDir(), "soloist")
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, rest)
	}
	return path
}

// isExecutable says whether path is a file Soloist may be, which the key
// step goes on to run.
func isExecutable(path string) bool {
	fi, err := os.Stat(expandHome(path))
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || fi.Mode()&0o111 != 0
}

// look reads what each step has set up, from the settings and the vault.
func (a *app) look() {
	ob := &a.ob
	cfg := a.settings.Spotify.withEnv()
	keep := a.vault()
	ob.have.app = cfg.ClientID != ""
	_, err := keep.Get(secret.SpotifyLogin)
	ob.have.login = err == nil
	ob.have.bin = cfg.SoloistBin != "" && isExecutable(cfg.SoloistBin)
	ob.have.key = soloistKey(keep) != ""
	ob.clientID, ob.redirect = cfg.ClientID, cfg.RedirectURI
	if ob.redirect == "" {
		ob.redirect = defaultRedirect
	}
}

// done says whether step i has what it sets up.
func (a *app) done(i int) bool {
	h := a.ob.have
	switch i {
	case stepApp:
		return h.app
	case stepLogin:
		return h.login
	case stepSoloist:
		return h.bin
	case stepKey:
		return h.key
	}
	return a.ob.paired
}

// firstUndone is the first step not done, or the last.
func (a *app) firstUndone() int {
	for i := range steps {
		if !a.done(i) {
			return i
		}
	}
	return steps - 1
}

// complete says whether every step is done.
func (a *app) complete() bool {
	for i := range steps {
		if !a.done(i) {
			return false
		}
	}
	return true
}

// restartNeeded says whether the app has to start again to play through
// what was set up.
func (a *app) restartNeeded() bool {
	return a.complete() && (a.ob.dirty || !a.live)
}

// openOnboarding opens the dialog at the first step not done.
func (a *app) openOnboarding() {
	a.look()
	a.ob.open = true
	a.goTo(a.firstUndone())
}

// goTo shows the controls of step i.
func (a *app) goTo(i int) {
	ob := &a.ob
	ob.step = i
	if i == stepPair {
		a.pairSoloist()
	}
}

// next goes on to the first step after i that isn't done.
func (a *app) next(i int) {
	for j := i + 1; j < steps; j++ {
		if !a.done(j) {
			a.goTo(j)
			return
		}
	}
	a.ob.step = -1
}

// closeOnboarding closes the dialog, and drops what was under way in it.
func (a *app) closeOnboarding() {
	ob := &a.ob
	ob.open = false
	a.stopOnboarding()
}

// stopOnboarding drops a login and a trial of the key under way, and
// forgets the key typed.
func (a *app) stopOnboarding() {
	ob := &a.ob
	if ob.stopLogin != nil {
		ob.stopLogin()
		ob.stopLogin = nil
	}
	if ob.stopKey != nil {
		ob.stopKey()
		ob.stopKey = nil
	}
	a.cancelDownload()
	ob.dl.confirm = false
	ob.loginGen++
	ob.keyGen++
	ob.loginWaiting, ob.loginErr = false, ""
	if ob.keyState == keyChecking {
		ob.keyState = keyIdle
	}
	ob.key = ""
}

// dismiss closes the dialog, and does not open it as the app starts
// again.
func (a *app) dismiss() {
	a.closeOnboarding()
	if !a.complete() {
		a.settings.OnboardingDismissed = true
		a.keepSettings()
	}
}

// saveApp keeps the client ID and the redirect URI typed.
func (a *app) saveApp() {
	ob := &a.ob
	id, redirect := strings.TrimSpace(ob.clientID), strings.TrimSpace(ob.redirect)
	if redirect == "" {
		redirect = defaultRedirect
	}
	if id == "" || strings.ContainsAny(id, " \t\n") {
		ob.appErr = "Client ID 不对，请从应用页面复制"
		return
	}
	// The login is to the Redirect URI as the listener's own app has it.
	if u, err := url.Parse(redirect); err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" {
		ob.appErr = "回调地址要写成 http://127.0.0.1:端口/路径"
		return
	}
	ob.appErr = ""
	was := a.settings.Spotify.withEnv()
	a.settings.Spotify.ClientID, a.settings.Spotify.RedirectURI = id, redirect
	// A login is of the app it was made to: another app's can't be used.
	if was.ClientID != id || was.RedirectURI != redirect {
		if err := a.vault().Delete(secret.SpotifyLogin); err != nil {
			log.Print(err)
		}
		ob.dirty = true
	}
	a.keepSettings()
	a.look()
	a.next(stepApp)
}

// startLogin opens the browser for the listener to log in, and keeps the
// login once they have.
func (a *app) startLogin() {
	ob := &a.ob
	cfg := a.settings.Spotify.withEnv()
	if cfg.ClientID == "" || ob.loginWaiting {
		return
	}
	ob.loginWaiting, ob.loginErr = true, ""
	ob.loginGen++
	gen := ob.loginGen
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	ob.stopLogin = cancel
	keep := a.vault()
	go func() {
		err := spotifyLogin(ctx, auth.Config{ClientID: cfg.ClientID, RedirectURI: cfg.RedirectURI}, keep)
		cancel()
		a.post(func() {
			if gen != ob.loginGen {
				return
			}
			ob.loginWaiting, ob.stopLogin = false, nil
			if err != nil {
				ob.loginErr = "登录失败，请再试一次"
				if errors.Is(err, auth.ErrDenied) {
					ob.loginErr = "你拒绝了授权"
				}
				return
			}
			ob.dirty = true
			a.look()
			a.next(stepLogin)
		})
	}()
}

// cancelLogin stops waiting for the browser.
func (a *app) cancelLogin() {
	ob := &a.ob
	if ob.stopLogin != nil {
		ob.stopLogin()
	}
	ob.loginGen++
	ob.loginWaiting, ob.stopLogin = false, nil
}

// cancelKey stops trying the key, which takes up to 30 seconds.
func (a *app) cancelKey() {
	ob := &a.ob
	if ob.stopKey != nil {
		ob.stopKey()
	}
	ob.keyGen++
	ob.keyState, ob.stopKey = keyIdle, nil
}

// pickSoloist asks for Soloist's executable, or the archive it came in,
// and keeps it.
func (a *app) pickSoloist() {
	path := chooseSoloist()
	if path == "" {
		return
	}
	ob := &a.ob
	if isArchive(path) {
		a.install(func(ctx context.Context, _ func(done, total int64)) (string, error) {
			return installSoloist(ctx, path, soloistHome())
		})
		return
	}
	if !isExecutable(path) {
		ob.binErr = "这不是可以运行的文件"
		return
	}
	a.useSoloist(path)
}

// useSoloist keeps path as Soloist's executable, and goes on.
func (a *app) useSoloist(path string) {
	ob := &a.ob
	ob.binErr = ""
	a.settings.Spotify.SoloistBin = path
	ob.dirty = true
	a.keepSettings()
	a.look()
	// Another Soloist may be paired, or not: the service runs it again.
	ob.paired = false
	a.soloistChanged()
	a.next(stepSoloist)
}

// askDownload asks the listener to confirm where Soloist is downloaded
// from, before it is.
func (a *app) askDownload() { a.ob.dl.confirm = true }

// startDownload downloads Soloist's build from Spotify's CDN, and keeps
// it once it runs.
func (a *app) startDownload() {
	a.ob.dl.confirm = false
	a.install(func(ctx context.Context, progress func(done, total int64)) (string, error) {
		return downloadSoloist(ctx, soloistHome(), progress)
	})
}

// install runs get off the main thread, which brings Soloist's executable
// and says how far it has got, and keeps what it brings.
func (a *app) install(get func(ctx context.Context, progress func(done, total int64)) (string, error)) {
	ob := &a.ob
	if ob.dl.busy {
		return
	}
	ob.binErr = ""
	gen := ob.dl.gen + 1
	ctx, cancel := context.WithCancel(context.Background())
	ob.dl = download{busy: true, total: -1, gen: gen, stop: cancel}
	go func() {
		bin, err := get(ctx, func(done, total int64) {
			a.post(func() {
				if gen == ob.dl.gen {
					ob.dl.done, ob.dl.total = done, total
				}
			})
		})
		cancel()
		a.post(func() {
			if gen != ob.dl.gen {
				return
			}
			ob.dl.busy, ob.dl.stop = false, nil
			switch {
			case err == nil:
				a.useSoloist(bin)
			case errors.Is(err, errNotSoloist):
				ob.binErr = "解压出来的文件运行不了，请再试，或自己选择"
			case errors.Is(err, errNoSoloist):
				ob.binErr = "压缩包里没有 Soloist"
			default:
				ob.binErr = "没能下载或解压，请再试一次"
			}
		})
	}()
}

// cancelDownload gives up on a download under way.
func (a *app) cancelDownload() {
	dl := &a.ob.dl
	if dl.stop != nil {
		dl.stop()
	}
	if dl.busy {
		dl.gen++
	}
	dl.busy, dl.stop = false, nil
}

// submitKey tries the key typed, and keeps it if it is good, or Soloist
// has no pairing to try it on.
func (a *app) submitKey() {
	ob := &a.ob
	key := strings.TrimSpace(ob.key)
	cfg := a.settings.Spotify.withEnv()
	if key == "" || ob.keyState == keyChecking || !ob.have.bin {
		return
	}
	ob.keyState = keyChecking
	ob.keyGen++
	gen := ob.keyGen
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	ob.stopKey = cancel
	keep := a.vault()
	go func() {
		err := checkKey(ctx, expandHome(cfg.SoloistBin), key, soloistDataDir())
		cancel()
		var state keyState
		switch {
		case err == nil:
			state = keyValid
		case errors.Is(err, soloist.ErrNotPaired):
			state = keyUnverified
		case errors.Is(err, soloist.ErrBadKey):
			state = keyInvalid
		default:
			state = keyFailed
		}
		// What went wrong is not said or logged, which may hold the key.
		if state == keyValid || state == keyUnverified {
			if err := keep.Set(secret.SoloistKey, []byte(key)); err != nil {
				log.Print(err)
				state = keyFailed
			}
		}
		a.post(func() {
			if gen != ob.keyGen {
				return
			}
			ob.keyState, ob.stopKey = state, nil
			if state == keyInvalid || state == keyFailed {
				return
			}
			ob.key = ""
			ob.dirty = true
			ob.paired = false
			a.look()
			a.soloistChanged()
			a.next(stepKey)
		})
	}()
}

// soloistChanged starts Soloist's service again, if it was started: it
// starts as the pairing step comes.
func (a *app) soloistChanged() {
	if a.ob.watching {
		a.restartSoloist()
	}
}

// restartSoloist starts Soloist's service as it is set up now, once it has
// what it needs.
func (a *app) restartSoloist() {
	ob := &a.ob
	cfg := a.settings.Spotify.withEnv()
	key := soloistKey(a.vault())
	if !ob.have.bin || key == "" {
		return
	}
	go func() {
		if err := startSoloist(cfg, key); err != nil {
			log.Print(err)
		}
	}()
}

// pairSoloist has Soloist start, and watches for it to log in: the
// listener pairs it in Spotify's app on their phone.
func (a *app) pairSoloist() {
	ob := &a.ob
	if ob.watching || !ob.have.bin || !ob.have.key {
		return
	}
	ob.watching = true
	cfg := a.settings.Spotify.withEnv()
	a.restartSoloist()
	watchSoloist(cfg, func() {
		a.post(func() {
			if !a.ob.have.key {
				return
			}
			a.ob.paired = true
			a.ob.dirty = true
			if a.ob.open && a.ob.step == stepPair {
				a.next(stepPair)
			}
		})
	})
}

// redo goes back to step i, which is done again alone: what the others
// set up stays.
func (a *app) redo(i int) {
	ob := &a.ob
	a.cancelLogin()
	switch i {
	case stepKey:
		ob.keyState, ob.key = keyIdle, ""
	case stepPair:
		ob.paired = false
	}
	a.goTo(i)
}

// signOut forgets the login, and shows the app as nobody's.
func (a *app) signOut() {
	if err := a.vault().Delete(secret.SpotifyLogin); err != nil {
		log.Print(err)
	}
	a.ob.have.login = false
	a.ob.dirty = true
	a.cancelLogin()
	a.signedIn(nil)
}

// keepSettings keeps the settings onboarding changed, which have nothing
// to apply.
func (a *app) keepSettings() {
	if err := a.settings.save(); err != nil {
		log.Print(err)
	}
}
