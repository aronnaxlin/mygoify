package app

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"mygoify/internal/agent"
)

// RunAgent runs the agent: what plays, on the session bus, while the
// window is closed (DESIGN.md §4.3). It stops as systemd stops it.
func RunAgent() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return agent.Run(ctx, agent.Config{SoloistDir: soloistDataDir(), Raise: func() { openApp() }})
}

// openApp starts the app, in a session of its own so that it outlives
// what started it; a running one brings its window up instead.
func openApp(args ...string) {
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
