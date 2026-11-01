package levels

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// dumped is a graph as pw-dump writes it: Spotify's own app and Soloist
// play a stream of the same name each.
const dumped = `[
  { "id": 31, "type": "PipeWire:Interface:Metadata", "props": { "metadata.name": "settings" },
    "metadata": [ { "subject": 0, "key": "clock.rate", "type": "", "value": 48000 },
                  { "subject": 0, "key": "clock.quantum", "type": "", "value": 1024 } ] },
  { "id": 98, "type": "PipeWire:Interface:Client",
    "info": { "props": { "application.name": "Spotify", "application.process.id": 100 } } },
  { "id": 99, "type": "PipeWire:Interface:Client",
    "info": { "props": { "application.name": "Spotify", "application.process.id": "200" } } },
  { "id": 89, "type": "PipeWire:Interface:Node",
    "info": { "state": "running", "props": { "node.name": "spotify", "media.class": "Stream/Output/Audio",
      "client.id": 98, "object.serial": 1350, "node.rate": "1/44100" } } },
  { "id": 90, "type": "PipeWire:Interface:Node",
    "info": { "state": "running", "props": { "node.name": "spotify", "media.class": "Stream/Output/Audio",
      "client.id": 99, "object.serial": 1340, "node.rate": "1/44100" } } },
  { "id": 91, "type": "PipeWire:Interface:Node",
    "info": { "props": { "node.name": "spotify", "media.class": "Stream/Input/Audio",
      "client.id": 99, "object.serial": 1341 } } },
  { "id": 93, "type": "PipeWire:Interface:Port",
    "info": { "direction": "output", "props": { "node.id": 90 }, "params": { "Latency": [
      { "direction": "Input", "minQuantum": 2.000000, "maxQuantum": 2.000000, "minRate": 384, "maxRate": 384, "minNs": 0, "maxNs": 0 },
      { "direction": "Output", "minQuantum": 0.000000, "maxQuantum": 0.000000, "minRate": 0, "maxRate": 0, "minNs": 0, "maxNs": 0 } ] } } }
]`

func dump(t *testing.T, g *graph, s string) {
	t.Helper()
	var b []pwObject
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		t.Fatal(err)
	}
	g.apply(b)
}

// Soloist's stream is the one its process plays, not any named spotify.
func TestTargetIsTheProcesssStream(t *testing.T) {
	g := newGraph()
	dump(t, g, dumped)
	n, ok := g.target(200)
	if !ok || n.serial != 1340 || n.rate != 44100 {
		t.Fatalf("target = %+v, %v; want serial 1340 at 44100", n, ok)
	}
	if _, ok := g.target(300); ok {
		t.Fatal("found a stream for a process that plays none")
	}
}

// Once Soloist closes its stream, as it does to pause, the tap lets it
// go, and takes the next it opens.
func TestTargetFollowsTheProcess(t *testing.T) {
	g := newGraph()
	dump(t, g, dumped)
	// An update of its state alone keeps it.
	dump(t, g, `[ { "id": 90, "type": "PipeWire:Interface:Node", "info": { "state": "idle" } } ]`)
	if n, _ := g.target(200); n.serial != 1340 {
		t.Fatalf("after an update, target = %+v", n)
	}
	dump(t, g, `[ { "id": 93, "info": null }, { "id": 90, "info": null } ]`)
	if n, ok := g.target(200); ok {
		t.Fatalf("after it went, target = %+v", n)
	}
	dump(t, g, `[ { "id": 90, "type": "PipeWire:Interface:Node",
	  "info": { "props": { "media.class": "Stream/Output/Audio", "client.id": 99, "object.serial": 1402, "node.rate": "1/48000" } } } ]`)
	if n, _ := g.target(200); n.serial != 1402 || n.rate != 48000 {
		t.Fatalf("after another opened, target = %+v", n)
	}
}

// Latency adds what PipeWire says it takes from the stream's port to the
// output: here two quanta and 384 samples.
func TestLatencyOfTheStream(t *testing.T) {
	g := newGraph()
	dump(t, g, dumped)
	n, _ := g.target(200)
	want := (2*1024 + 384) * time.Second / 48000
	if got := g.latency(n); got != want {
		t.Fatalf("latency = %v, want %v", got, want)
	}
	other, _ := g.target(100)
	if got := g.latency(other); got != 0 {
		t.Fatalf("latency of a stream with no ports = %v", got)
	}
}

// Frames split anywhere on the way down the pipe come out whole.
func TestPCMSplitAnywhere(t *testing.T) {
	var want []float32
	var raw []byte
	for i := range 49 {
		v := float32(i) / 7
		want = append(want, v)
		raw = binary.NativeEndian.AppendUint32(raw, math.Float32bits(v))
	}
	want = want[:48] // 24 whole stereo frames, and half of one
	var p pcm
	var got []float32
	for len(raw) > 0 {
		k := min(len(raw), 5)
		s := p.decode(raw[:k], 2)
		if len(s)%2 != 0 {
			t.Fatalf("decoded half a frame: %d samples", len(s))
		}
		got = append(got, s...)
		raw = raw[k:]
	}
	if !slices.Equal(got, want) {
		t.Fatalf("decoded %v\nwant %v", got, want)
	}
}

// Tap hears a tone that another process plays, into an output of its own
// so no one else does, and stops hearing it, leaving no tool running,
// once stopped. It needs PipeWire, so it runs only with MYGOIFY_PIPEWIRE.
func TestTapHearsAProcess(t *testing.T) {
	if os.Getenv("MYGOIFY_PIPEWIRE") == "" {
		t.Skip("set MYGOIFY_PIPEWIRE to tap a tone through the session's PipeWire")
	}
	out, err := exec.Command("pactl", "load-module", "module-null-sink", "sink_name=mygoify-levels-test").Output()
	if err != nil {
		t.Fatal(err)
	}
	defer exec.Command("pactl", "unload-module", strings.TrimSpace(string(out))).Run()

	play := exec.Command("pw-play", "--raw", "--format", "f32", "--rate", "44100", "--channels", "2",
		"--target", "mygoify-levels-test", "-")
	in, _ := play.StdinPipe()
	if err := play.Start(); err != nil {
		t.Fatal(err)
	}
	defer play.Wait()
	defer play.Process.Kill()
	go func() {
		// A 1 kHz sine, for as long as the test reads it.
		var buf []byte
		for i := 0; ; i++ {
			v := math.Float32bits(float32(0.5 * math.Sin(2*math.Pi*1000*float64(i)/44100)))
			buf = binary.NativeEndian.AppendUint32(binary.NativeEndian.AppendUint32(buf, v), v)
			if len(buf) >= 4096 {
				if _, err := in.Write(buf); err != nil {
					return
				}
				buf = buf[:0]
			}
		}
	}()

	m := NewMeter()
	stop, err := Tap(play.Process.Pid, m)
	if err != nil {
		t.Fatal(err)
	}
	var lv [Bands]float32
	heard := false
	for deadline := time.Now().Add(5 * time.Second); !heard && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		lv, heard = m.Levels()
	}
	if !heard {
		stop()
		t.Fatal("heard nothing")
	}
	t.Logf("heard %.2f, delay %d frames", lv, m.delay)
	m.findEdges(44100)
	band := 0
	for b := range Bands {
		if m.edges[b]*44100/fftSize <= 1000 {
			band = b
		}
	}
	if at, _ := loudest(lv); at < band-1 || at > band+1 {
		t.Errorf("loudest band %d of %v, want about %d", at, lv, band)
	}
	stop()
	time.Sleep(2 * stale)
	if _, ok := m.Levels(); ok {
		t.Error("still hears it once stopped")
	}
	for _, kid := range children(t) {
		if strings.Contains(kid, "(pw-record)") || strings.Contains(kid, "(pw-dump)") {
			t.Errorf("left running: %s", kid)
		}
	}
	in.Close()
}

// children names the processes this one started that are still there,
// running or not reaped.
func children(t *testing.T) []string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	var kids []string
	me := strconv.Itoa(os.Getpid())
	for _, e := range entries {
		f, err := os.Open("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(f)
		f.Close()
		// pid (comm) state ppid ...
		s := string(b)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		fields := strings.Fields(s[i+1:])
		if len(fields) > 1 && fields[1] == me {
			kids = append(kids, s[:i+1])
		}
	}
	return kids
}
