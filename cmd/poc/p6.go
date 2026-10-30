package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// dialSoloist connects to the WebSocket of the Soloist whose data
// directory is ours, at the address it wrote there.
func dialSoloist(ctx context.Context) (*websocket.Conn, error) {
	addr, err := os.ReadFile(filepath.Join(dataDir, "soloist/ws.addr"))
	if err != nil {
		return nil, err
	}
	port, err := os.ReadFile(filepath.Join(dataDir, "soloist/ws.port"))
	if err != nil {
		return nil, err
	}
	ws, _, err := websocket.Dial(ctx, "ws://"+strings.TrimSpace(string(addr))+":"+strings.TrimSpace(string(port)), nil)
	return ws, err
}

func command(ctx context.Context, ws *websocket.Conn, name string) error {
	b, _ := json.Marshal(map[string]string{"type": "command", "command": name})
	return ws.Write(ctx, websocket.MessageText, b)
}

// anchor is a position Soloist reported: where playback was at its
// timestamp, and when it reached us.
type anchor struct {
	uri      string
	pos, ts  float64 // ms
	speed    float64
	received float64 // ms, by our clock
}

// p6 plays a minute and measures how well the position Soloist reports
// interpolates: each anchor against the one before it extrapolated, and
// how far its timestamps are from our clock. It samples Soloist's memory
// and CPU meanwhile, for P7.
func p6() error {
	const length = 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), length+30*time.Second)
	defer cancel()
	ws, err := dialSoloist(ctx)
	if err != nil {
		return err
	}
	defer ws.CloseNow()

	anchors := make(chan anchor, 64)
	go func() {
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				close(anchors)
				return
			}
			now := float64(time.Now().UnixMicro()) / 1000
			var ev struct {
				Type     string
				Status   string
				Item     struct{ URI string }
				Position struct {
					PositionMs  float64 `json:"position_ms"`
					TimestampMs float64 `json:"timestamp_ms"`
					Speed       float64
				}
			}
			if json.Unmarshal(data, &ev) != nil || ev.Position.TimestampMs == 0 {
				continue
			}
			if ev.Type == "playback_state" || ev.Type == "position_sync" {
				anchors <- anchor{ev.Item.URI, ev.Position.PositionMs, ev.Position.TimestampMs, ev.Position.Speed, now}
			}
		}
	}()

	if err := command(ctx, ws, "play"); err != nil {
		return err
	}
	fmt.Printf("Playing for %v…\n", length)

	var (
		all      []anchor
		drifts   []float64 // ms: an anchor against the last one extrapolated
		skews    []float64 // ms: our clock at receipt minus Soloist's position extrapolated to it
		pss      []float64 // MB
		cpuStart = cpuTicks()
		start    = time.Now()
	)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	deadline := time.After(length)
loop:
	for {
		select {
		case a, ok := <-anchors:
			if !ok {
				return fmt.Errorf("Soloist closed the connection")
			}
			if a.speed == 0 {
				continue
			}
			if n := len(all); n > 0 {
				prev := all[n-1]
				if prev.uri == a.uri && prev.speed > 0 {
					predicted := prev.pos + (a.ts-prev.ts)*prev.speed
					drifts = append(drifts, a.pos-predicted)
				}
			}
			skews = append(skews, a.received-a.ts)
			all = append(all, a)
		case <-tick.C:
			command(ctx, ws, "get_state")
			if int(time.Since(start).Seconds())%5 == 0 {
				pss = append(pss, soloistPSS())
			}
		case <-deadline:
			break loop
		}
	}
	elapsed := time.Since(start)
	cpu := float64(cpuTicks()-cpuStart) / 100 / elapsed.Seconds() * 100 // USER_HZ is 100
	command(ctx, ws, "pause")

	fmt.Printf("anchors: %d\n", len(all))
	report("drift of an anchor against the one before, extrapolated (ms)", drifts)
	report("our clock minus Soloist's timestamp at receipt (ms)", skews)
	report("Soloist's PSS while playing, every 5 s (MB)", pss)
	fmt.Printf("Soloist's CPU while playing: %.2f%% of a core\n", cpu)
	fmt.Println("Paused.")
	return nil
}

func report(name string, v []float64) {
	if len(v) == 0 {
		fmt.Printf("%s: no samples\n", name)
		return
	}
	s := slices.Clone(v)
	slices.Sort(s)
	var abs float64
	for _, x := range s {
		abs = math.Max(abs, math.Abs(x))
	}
	fmt.Printf("%s: n=%d min=%.1f median=%.1f max=%.1f max|x|=%.1f\n",
		name, len(s), s[0], s[len(s)/2], s[len(s)-1], abs)
}

// soloistPIDs lists the processes running Soloist: the player and its
// crash handler.
func soloistPIDs() []string {
	var pids []string
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if strings.TrimSpace(string(comm)) == "soloist" {
			pids = append(pids, e.Name())
		}
	}
	return pids
}

// soloistPSS sums the proportional set size of Soloist's processes, in MB.
func soloistPSS() float64 {
	var kb float64
	for _, pid := range soloistPIDs() {
		f, err := os.Open(filepath.Join("/proc", pid, "smaps_rollup"))
		if err != nil {
			continue
		}
		s := bufio.NewScanner(f)
		for s.Scan() {
			if f := strings.Fields(s.Text()); len(f) >= 2 && f[0] == "Pss:" {
				v, _ := strconv.ParseFloat(f[1], 64)
				kb += v
			}
		}
		f.Close()
	}
	return kb / 1024
}

// cpuTicks sums the user and system time of Soloist's processes, in clock
// ticks.
func cpuTicks() int64 {
	var t int64
	for _, pid := range soloistPIDs() {
		b, err := os.ReadFile(filepath.Join("/proc", pid, "stat"))
		if err != nil {
			continue
		}
		// The fields after the command name, which may hold spaces, in
		// parentheses: utime and stime are the 12th and 13th of them.
		f := strings.Fields(string(b[strings.LastIndexByte(string(b), ')')+1:]))
		u, _ := strconv.ParseInt(f[11], 10, 64)
		s, _ := strconv.ParseInt(f[12], 10, 64)
		t += u + s
	}
	return t
}
