// Package agent is MyGoify's agent: what stays while the window is closed,
// beside Soloist, so that the media keys, playerctl and the desktop's media
// controls still drive what plays (DESIGN.md §4.3). It follows Soloist
// over its WebSocket and shows what plays as an MPRIS player, and loads no
// GTK: it costs little to keep.
package agent

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"

	"mygoify/internal/soloist"
)

// Config is what the agent follows, and how it opens the app.
type Config struct {
	// SoloistDir is Soloist's data directory, where it writes the
	// address of its WebSocket.
	SoloistDir string
	// Raise opens the app's window, as the desktop asks the player to.
	Raise func()
	// Tray, if set, is the agent's icon in the desktop's tray.
	Tray *TrayItem
}

// Run follows Soloist and shows what plays on the session bus until ctx
// ends. It fails if another agent runs.
func Run(ctx context.Context, cfg Config) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	b := soloist.New(soloist.Config{DataDir: cfg.SoloistDir, NoLevels: true})
	m, err := exportMPRIS(conn, b, keptVolume(cfg.SoloistDir), cfg.Raise)
	if err != nil {
		return err
	}
	b.Watch(m.update)
	if cfg.Tray != nil {
		if err := exportTray(conn, *cfg.Tray); err != nil {
			log.Printf("agent: tray: %v", err)
		}
	}
	<-ctx.Done()
	return nil
}

// keptVolume is the volume Soloist starts at, as the app keeps it beside
// its data directory, from 0 to 1; 0.4, Soloist's own, if none is kept.
func keptVolume(dir string) float64 {
	b, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "soloist.volume"))
	if err != nil {
		return 0.4
	}
	_, v, _ := strings.Cut(strings.TrimSpace(string(b)), "=")
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0.4
	}
	return float64(max(0, min(100, n))) / 100
}

var _ Controls = (*soloist.Backend)(nil)
