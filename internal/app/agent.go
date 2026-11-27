package app

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/egoist/mygo"

	"mygoify/internal/agent"
	"mygoify/internal/omarchy"
	"mygoify/locales"
)

// RunAgent runs the agent: what plays, on the session bus, while the
// window is closed (DESIGN.md §4.3). It stops as systemd stops it.
func RunAgent() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// On Omarchy its icon is in Omarchy's bar, where a click opens the mini
	// player (DESIGN.md §4.3).
	cfg := agent.Config{SoloistDir: soloistDataDir(), Raise: func() { openApp() }}
	if exe, err := os.Executable(); err == nil && omarchy.Detect() {
		go func() {
			if err := omarchy.KeepWidget(exe); err != nil {
				log.Print(err)
			}
		}()
	} else {
		// Elsewhere, in the tray: a click opens the mini player there too.
		msgs := locales.In(appLocale())
		cfg.Tray = &agent.TrayItem{
			Title:    "MyGoify",
			Icon:     trayColor,
			Activate: func() { openApp("panel") },
			Items: []agent.MenuItem{
				{Label: msgs.MiniPlayer(), Chosen: func() { openApp("panel") }},
				{Label: msgs.OpenMyGoify(), Chosen: func() { openApp() }},
				{},
				// Quitting stops Soloist, and the agent with it.
				{Label: msgs.MenuQuit("MyGoify"), Chosen: stopSoloist},
			},
		}
	}
	return agent.Run(ctx, cfg)
}

// appLocale is the language the listener chose for the app, as the app
// keeps it, or "" for the system's.
func appLocale() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dir, mygo.App.Name(), "locale"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// stopSoloist stops Soloist's service, and the agent bound to it.
func stopSoloist() {
	if err := exec.Command("systemctl", "--user", "stop", "mygoify-soloist.service").Run(); err != nil {
		log.Print(err)
	}
}

// openApp starts the app, in a session of its own so that it outlives
// what started it; a running one brings its window up instead.
var openApp = func(args ...string) {
	exe, err := os.Executable()
	if err != nil {
		log.Print(err)
		return
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		log.Print(err)
		return
	}
	go cmd.Wait()
}
