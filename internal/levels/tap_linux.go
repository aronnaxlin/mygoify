package levels

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// On Linux, PipeWire hands any client the output of another's stream:
// pw-record, told to take Soloist's stream node as its target, gets what
// Soloist plays before the system mixes it, as raw floats down a pipe,
// and pw-dump --monitor tells which node that is as streams come and go.
// Two of PipeWire's own tools, rather than its protocol spoken in Go,
// which is a large one to get right and changes, and cgo is out; they
// come with PipeWire wherever it is. Nothing touches the disk: the
// samples go from the pipe to the meter.

// recordProps keep pw-record from hearing anything but Soloist: without
// a fallback, or a reconnect once its node goes, it ends rather than
// take the microphone. Passive, it doesn't keep the output awake once
// Soloist pauses.
const recordProps = "{ node.name = mygoify-levels node.passive = true node.dont-fallback = true node.dont-reconnect = true }"

// Tap feeds m what process pid plays until stop is called, which the
// caller does before tapping again. It follows the process from stream
// to stream: Soloist, paused, has none, and opens another to play again.
// Should the app die first, the tools die of the pipe they write to.
func Tap(pid int, m *Meter) (stop func(), err error) {
	for _, tool := range []string{"pw-dump", "pw-record"} {
		if _, err := exec.LookPath(tool); err != nil {
			return nil, ErrUnsupported
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	dump := exec.CommandContext(ctx, "pw-dump", "--monitor", "--no-colors")
	out, err := dump.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := dump.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("levels: pw-dump: %w", err)
	}
	// pw-dump writes the whole graph first, then what changes in it.
	batches := make(chan []pwObject)
	go func() {
		defer close(batches)
		dec := json.NewDecoder(out)
		for {
			var b []pwObject
			if dec.Decode(&b) != nil {
				return
			}
			select {
			case batches <- b:
			case <-ctx.Done():
				return
			}
		}
	}()
	end := func() {
		cancel()
		for range batches {
		}
		dump.Wait()
	}

	g := newGraph()
	select {
	case b, ok := <-batches:
		if !ok {
			end()
			return nil, errors.New("levels: PipeWire doesn't answer")
		}
		g.apply(b)
	case <-time.After(5 * time.Second):
		end()
		return nil, errors.New("levels: PipeWire doesn't answer")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer end()
		var rec *recorder
		defer func() { rec.stop() }()
		// gave is the node whose recorder ended by itself, not tried again.
		gave := 0
		var latency time.Duration
		for {
			n, ok := g.target(pid)
			if rec != nil && (!ok || rec.serial != n.serial) {
				rec.stop()
				rec = nil
			}
			if ok && rec == nil && n.serial != gave {
				rec, latency = record(n, m), -1
			}
			if rec != nil {
				if l := g.latency(n); l != latency {
					latency = l
					m.SetFormat(n.rate, l)
				}
			}
			var ended chan struct{}
			if rec != nil {
				ended = rec.done
			}
			select {
			case b, ok := <-batches:
				if !ok {
					// PipeWire went away.
					return
				}
				g.apply(b)
			case <-ended:
				rec.stop()
				gave, rec = rec.serial, nil
			case <-ctx.Done():
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}, nil
}

// recorder is a pw-record of one stream node.
type recorder struct {
	serial int
	cmd    *exec.Cmd
	done   chan struct{} // closed once it ends and is reaped
}

// record starts hearing node n, in the format it plays, so nothing is
// resampled; it is mixed down to two channels.
func record(n node, m *Meter) *recorder {
	r := &recorder{serial: n.serial, done: make(chan struct{})}
	r.cmd = exec.Command("pw-record", "--raw", "--format", "f32",
		"--rate", strconv.Itoa(n.rate), "--channels", "2",
		"--target", strconv.Itoa(n.serial), "-P", recordProps, "-")
	out, err := r.cmd.StdoutPipe()
	if err == nil {
		err = r.cmd.Start()
	}
	if err != nil {
		close(r.done)
		r.cmd = nil
		return r
	}
	go func() {
		defer close(r.done)
		var p pcm
		buf := make([]byte, 1<<14)
		for {
			k, err := out.Read(buf)
			if k > 0 {
				m.Feed(p.decode(buf[:k], 2), 2)
			}
			if err != nil {
				break
			}
		}
		r.cmd.Wait()
	}()
	return r
}

func (r *recorder) stop() {
	if r == nil {
		return
	}
	if r.cmd != nil {
		r.cmd.Process.Kill()
	}
	<-r.done
}

// pcm turns what comes down the pipe, native floats in pieces of any
// size, into whole frames of samples.
type pcm struct {
	rest []byte
	out  []float32
}

func (p *pcm) decode(b []byte, channels int) []float32 {
	p.rest = append(p.rest, b...)
	n := len(p.rest) / (4 * channels) * (4 * channels)
	p.out = p.out[:0]
	for i := 0; i < n; i += 4 {
		p.out = append(p.out, math.Float32frombits(binary.NativeEndian.Uint32(p.rest[i:])))
	}
	p.rest = append(p.rest[:0], p.rest[n:]...)
	return p.out
}

// pwObject is an object of pw-dump's, as much of it as Tap reads. One
// whose info is null has gone.
type pwObject struct {
	ID       int             `json:"id"`
	Type     string          `json:"type"`
	Info     json.RawMessage `json:"info"`
	Metadata []struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	} `json:"metadata"`
}

type pwInfo struct {
	Direction string   `json:"direction"`
	Props     *pwProps `json:"props"`
	Params    struct {
		Latency []struct {
			Direction string  `json:"direction"`
			Quantum   float64 `json:"minQuantum"`
			Rate      float64 `json:"minRate"`
			Ns        float64 `json:"minNs"`
		} `json:"Latency"`
	} `json:"params"`
}

type pwProps struct {
	Class  string `json:"media.class"`
	Client num    `json:"client.id"`
	PID    num    `json:"application.process.id"`
	Serial num    `json:"object.serial"`
	Rate   string `json:"node.rate"` // as "1/44100"
	Node   num    `json:"node.id"`
}

// num is a number that some clients give as a string.
type num int

func (n *num) UnmarshalJSON(b []byte) error {
	v, err := strconv.Atoi(strings.Trim(string(b), `"`))
	if err == nil {
		*n = num(v)
	}
	return nil
}

// node is a stream that plays.
type node struct {
	id, client, pid, serial, rate int
}

// port is an output port of one, and how long what leaves it takes to be
// heard: so many quanta, samples and nanoseconds.
type port struct {
	node            int
	quanta, samples float64
	ns              float64
}

// graph is what Tap knows of PipeWire's, from pw-dump.
type graph struct {
	pids          map[int]int // a client's process
	nodes         map[int]node
	ports         map[int]port
	rate, quantum int // the clock's settings
	forced        struct{ rate, quantum int }
}

func newGraph() *graph {
	return &graph{pids: map[int]int{}, nodes: map[int]node{}, ports: map[int]port{}, rate: 48000, quantum: 1024}
}

func (g *graph) apply(objs []pwObject) {
	for _, o := range objs {
		if string(o.Info) == "null" {
			delete(g.pids, o.ID)
			delete(g.nodes, o.ID)
			delete(g.ports, o.ID)
			continue
		}
		var info pwInfo
		if len(o.Info) > 0 && json.Unmarshal(o.Info, &info) != nil {
			continue
		}
		p := info.Props
		switch o.Type {
		case "PipeWire:Interface:Client":
			if p != nil && p.PID != 0 {
				g.pids[o.ID] = int(p.PID)
			}
		case "PipeWire:Interface:Node":
			if p == nil {
				// An update of what isn't its properties.
				continue
			}
			if p.Class != "Stream/Output/Audio" {
				delete(g.nodes, o.ID)
				continue
			}
			n := node{id: o.ID, client: int(p.Client), pid: int(p.PID), serial: int(p.Serial), rate: 48000}
			if _, r, ok := strings.Cut(p.Rate, "/"); ok {
				if v, err := strconv.Atoi(r); err == nil && v > 0 {
					n.rate = v
				}
			}
			g.nodes[o.ID] = n
		case "PipeWire:Interface:Port":
			if info.Direction != "output" {
				continue
			}
			pt, ok := g.ports[o.ID]
			if p != nil {
				pt.node, ok = int(p.Node), true
			}
			for _, l := range info.Params.Latency {
				// Toward the output: what it takes to reach the ear.
				if l.Direction == "Input" {
					pt.quanta, pt.samples, pt.ns = l.Quantum, l.Rate, l.Ns
				}
			}
			if ok {
				g.ports[o.ID] = pt
			}
		case "PipeWire:Interface:Metadata":
			for _, md := range o.Metadata {
				var v num
				v.UnmarshalJSON(md.Value)
				switch md.Key {
				case "clock.rate":
					g.rate = int(v)
				case "clock.quantum":
					g.quantum = int(v)
				case "clock.force-rate":
					g.forced.rate = int(v)
				case "clock.force-quantum":
					g.forced.quantum = int(v)
				}
			}
		}
	}
}

// target is the stream process pid plays, the newest if it has more.
func (g *graph) target(pid int) (node, bool) {
	var best node
	for _, n := range g.nodes {
		if (n.pid == pid || g.pids[n.client] == pid) && n.serial > best.serial {
			best = n
		}
	}
	return best, best.serial != 0
}

// latency is how long what n plays takes from leaving it to being heard:
// a quantum or two in the output's buffer through the speakers, and the
// codec's delay too over Bluetooth, as PipeWire works it out along the
// way. A quantum is the graph's, unless forced: what the output really
// runs at isn't in pw-dump, but it is near enough.
func (g *graph) latency(n node) time.Duration {
	rate, quantum := g.rate, g.quantum
	if g.forced.rate > 0 {
		rate = g.forced.rate
	}
	if g.forced.quantum > 0 {
		quantum = g.forced.quantum
	}
	if rate <= 0 {
		return 0
	}
	var most time.Duration
	for _, pt := range g.ports {
		if pt.node != n.id {
			continue
		}
		s := (pt.quanta*float64(quantum) + pt.samples) / float64(rate)
		most = max(most, time.Duration(s*float64(time.Second)+pt.ns))
	}
	return most
}
