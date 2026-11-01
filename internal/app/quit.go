package app

import (
	"context"
	"log"
	"sync"
	"time"

	"mygoify/internal/soloist"
)

// quitModes are what quitting the app does with the music (DESIGN.md
// §4.2), with their names: stop it and Soloist, pause it and leave
// Soloist running, or play on.
var quitModes = []struct{ id, label, desc string }{
	{"stop", "停止并关闭 Soloist", "从手机投放到这台电脑的播放也会停止"},
	{"pause", "暂停", "Soloist 留在后台，约占 60 MB 内存，下次打开更快"},
	{"play", "继续播放", "退出后，要回到 App 才能暂停或切歌"},
}

// pauser is a backend that can pause as the app quits: Soloist's.
type pauser interface {
	PauseAndWait(ctx context.Context) error
}

// service is Soloist's service, as the app starts and stops it.
type service interface {
	Start(atLogin bool) error
	Stop() error
}

// soloistRunner runs a soloist.Service one command at a time: the app
// starts it as it starts, and again as the settings change.
type soloistRunner struct {
	mu  sync.Mutex
	svc *soloist.Service
}

func (r *soloistRunner) Start(atLogin bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.svc.AtLogin = atLogin
	return r.svc.Start()
}

func (r *soloistRunner) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.svc.Stop()
}

// leaveWait is how long quitting waits for Soloist to pause, and again to
// stop.
var leaveWait = 2 * time.Second

// leaveSoloist does with the music what the settings say quitting does,
// after the windows have gone.
func (a *app) leaveSoloist() {
	mode := a.settings.Quit
	if mode == "play" || a.relaunching {
		return
	}
	if p, ok := a.backend.(pauser); ok {
		ctx, cancel := context.WithTimeout(context.Background(), leaveWait)
		if err := p.PauseAndWait(ctx); err != nil {
			log.Print(err)
		}
		cancel()
	}
	if mode != "stop" || a.svc == nil {
		return
	}
	done := make(chan error, 1)
	go func() { done <- a.svc.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			log.Print(err)
		}
	case <-time.After(leaveWait):
		log.Print("soloist: still stopping as the app quit")
	}
}

// changeAtLogin starts Soloist at login, or no longer, as the settings
// now say; the Soloist that plays plays on.
func (a *app) changeAtLogin() {
	if a.svc == nil {
		return
	}
	at := a.settings.Spotify.SoloistAtLogin
	go func() {
		if err := a.svc.Start(at); err != nil {
			log.Print(err)
		}
	}()
}
