// Package soloist plays through Spotify Soloist, Spotify's own headless
// player: Backend follows it over its local WebSocket and sends it the
// listener's commands. What to play starts over the WebSocket too when it
// can: a context from its start. From a track of a context, or a list of
// tracks, it starts through the Web API, as the WebSocket cannot
// (DESIGN.md §4.2).
package soloist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"mygoify/internal/music"
	"mygoify/internal/playback"
	"mygoify/internal/spotify"
)

// Config is the Soloist to play through.
type Config struct {
	// DataDir is its data directory, where it writes the address of its
	// WebSocket (ws.addr and ws.port).
	DataDir string
	// DeviceName is its name in Spotify Connect, by which the Web API's
	// list of devices names it.
	DeviceName string
	// API starts playback on it, as the WebSocket cannot.
	API *spotify.Client
	// DeviceFile, if set, keeps its ID in the Web API across restarts:
	// finding it again is a call to the Web API each time the app starts.
	DeviceFile string
}

// Backend plays through a Soloist. It connects as Watch is called, and
// again whenever the connection drops, as Soloist restarts.
type Backend struct {
	cfg Config

	mu       sync.Mutex
	watch    func(playback.State)
	conn     *websocket.Conn
	status   string // idle, playing, paused or buffering
	item     *entity
	ctxURI   string
	position position
	options  options
	previous []queueEntry // the most recent first, as Soloist lists them
	upcoming []queueEntry
	active   bool
	device   string // its ID in the Web API, once found
	err      string
	states   chan playback.State
}

func New(cfg Config) *Backend { return &Backend{cfg: cfg} }

// Watch reports each state to fn, in order, and starts following Soloist.
func (b *Backend) Watch(fn func(playback.State)) {
	b.mu.Lock()
	b.watch = fn
	b.states = make(chan playback.State, 64)
	states := b.states
	b.mu.Unlock()
	go func() {
		for st := range states {
			fn(st)
		}
	}()
	go b.follow(context.Background())
}

// follow keeps a connection to Soloist, reading what it reports, and
// connects again, a little later each time, while it can't.
func (b *Backend) follow(ctx context.Context) {
	wait := 250 * time.Millisecond
	for ctx.Err() == nil {
		err := b.session(ctx)
		if err == nil {
			wait = 250 * time.Millisecond
		}
		time.Sleep(wait)
		wait = min(wait*2, 5*time.Second)
	}
}

// addr reads the address Soloist's WebSocket listens on.
func (b *Backend) addr() (string, error) {
	host, err := os.ReadFile(filepath.Join(b.cfg.DataDir, "ws.addr"))
	if err != nil {
		return "", err
	}
	port, err := os.ReadFile(filepath.Join(b.cfg.DataDir, "ws.port"))
	if err != nil {
		return "", err
	}
	return "ws://" + strings.TrimSpace(string(host)) + ":" + strings.TrimSpace(string(port)), nil
}

// session connects, asks for where playback is, and reads until the
// connection drops.
func (b *Backend) session(ctx context.Context) error {
	addr, err := b.addr()
	if err != nil {
		return err
	}
	dial, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, _, err := websocket.Dial(dial, addr, nil)
	cancel()
	if err != nil {
		return err
	}
	conn.SetReadLimit(16 << 20)
	defer conn.CloseNow()
	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		if b.conn == conn {
			b.conn = nil
		}
		b.mu.Unlock()
	}()
	for _, cmd := range []string{"get_auth_state", "get_state", "get_queue"} {
		if err := b.send(ctx, map[string]any{"command": cmd}); err != nil {
			return err
		}
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return nil // dropped: connect again
		}
		if debug {
			log.Printf("soloist: < %s", summary(data))
		}
		b.handle(data)
	}
}

// event is a message of Soloist's, of any type: each sets what it has.
type event struct {
	Type     string       `json:"type"`
	Status   string       `json:"status"`
	Item     *entity      `json:"item"`
	Context  *entity      `json:"context"`
	Position *position    `json:"position"`
	Options  *options     `json:"options"`
	IsActive *bool        `json:"is_active"`
	Previous []queueEntry `json:"previous"`
	Upcoming []queueEntry `json:"upcoming"`
	Message  string       `json:"message"`
	LoggedIn *bool        `json:"logged_in"`
}

type entity struct {
	URI         string `json:"uri"`
	EntityType  string `json:"entity_type"`
	Decorations struct {
		Identity struct {
			Name string `json:"name"`
		} `json:"identity"`
		VisualIdentity struct {
			Cover []struct {
				URL  string `json:"url"`
				Size string `json:"size"`
			} `json:"cover"`
		} `json:"visual_identity"`
		Parent *struct {
			Entity *entity `json:"entity"`
		} `json:"parent"`
		Creators []struct {
			Entity *entity `json:"entity"`
		} `json:"creators"`
		Playback struct {
			DurationMs int64 `json:"duration_ms"`
		} `json:"playback"`
	} `json:"decorations"`
}

type position struct {
	PositionMs  int64   `json:"position_ms"`
	TimestampMs int64   `json:"timestamp_ms"`
	Speed       float64 `json:"speed"`
}

type options struct {
	Shuffle bool   `json:"shuffle"`
	Repeat  string `json:"repeat"` // off, context or track
}

type queueEntry struct {
	UID    string  `json:"uid"`
	Source string  `json:"source"` // context, queue or autoplay
	Item   *entity `json:"item"`
}

// coverSizes are the widths of the pictures Soloist names by size: those
// of Spotify's image CDN.
var coverSizes = map[string]int{"small": 64, "default": 300, "large": 640, "xlarge": 640}

// id is the last part of a Spotify URI: spotify:track:ID is ID.
func id(uri string) string { return uri[strings.LastIndexByte(uri, ':')+1:] }

// track is what the app shows of an entity Soloist plays.
func (e *entity) track() music.Track {
	d := &e.Decorations
	t := music.Track{ID: id(e.URI), Name: d.Identity.Name, Duration: d.Playback.DurationMs}
	for _, c := range d.Creators {
		if c.Entity != nil {
			t.Artists = append(t.Artists, music.Artist{ID: id(c.Entity.URI), Name: c.Entity.Decorations.Identity.Name})
		}
	}
	if p := d.Parent; p != nil && p.Entity != nil {
		t.Album = music.Album{ID: id(p.Entity.URI), Name: p.Entity.Decorations.Identity.Name}
	}
	seen := map[string]bool{}
	for _, c := range d.VisualIdentity.Cover {
		if !seen[c.URL] {
			seen[c.URL] = true
			px := coverSizes[c.Size]
			t.Album.Images = append(t.Album.Images, music.Image{URL: c.URL, Width: px, Height: px})
		}
	}
	return t
}

// handle takes in what Soloist sent, and reports the state it leaves.
func (b *Backend) handle(data []byte) {
	var ev event
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch ev.Type {
	case "playback_state", "playback_changed":
		if ev.Status != "" {
			b.status = ev.Status
		}
	case "error":
		b.err = ev.Message
	case "auth_state":
		if ev.LoggedIn != nil && !*ev.LoggedIn {
			b.err = "Soloist 未登录"
		}
	case "queue_changed":
		b.previous, b.upcoming = ev.Previous, ev.Upcoming
	case "command_result":
		return
	}
	if ev.Item != nil {
		if ev.Item.URI == "" {
			b.item = nil
		} else {
			b.item = ev.Item
		}
	}
	if ev.Context != nil {
		b.ctxURI = ev.Context.URI
	}
	if ev.Position != nil {
		b.position = *ev.Position
	}
	if ev.Options != nil {
		b.options = *ev.Options
	}
	if ev.IsActive != nil {
		b.active = *ev.IsActive
	}
	if ev.Type != "error" && ev.Type != "auth_state" {
		b.err = ""
	}
	b.emit()
}

// state is where playback is, as the app shows it. b.mu is held.
func (b *Backend) state() playback.State {
	st := playback.State{
		At:       -1,
		Playing:  b.status == "playing" || b.status == "buffering",
		Loading:  b.status == "buffering",
		Position: time.Duration(b.position.PositionMs) * time.Millisecond,
		Since:    time.UnixMilli(b.position.TimestampMs),
		Shuffle:  b.options.Shuffle,
		Err:      b.err,
	}
	switch b.options.Repeat {
	case "context":
		st.Repeat = playback.RepeatAll
	case "track":
		st.Repeat = playback.RepeatOne
	}
	if b.item == nil {
		return st
	}
	for _, e := range slices.Backward(b.previous) {
		if e.Item != nil && e.Item.URI != "" {
			st.Queue = append(st.Queue, e.Item.track())
		}
	}
	st.At = len(st.Queue)
	st.Queue = append(st.Queue, b.item.track())
	for _, e := range b.upcoming {
		if e.Item != nil && e.Item.URI != "" {
			st.Queue = append(st.Queue, e.Item.track())
		}
	}
	return st
}

func (b *Backend) emit() {
	if b.states != nil {
		b.states <- b.state()
	}
}

var errNotConnected = errors.New("soloist: not connected")

// send sends a command over the WebSocket.
func (b *Backend) send(ctx context.Context, cmd map[string]any) error {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if conn == nil {
		return errNotConnected
	}
	cmd["type"] = "command"
	data, _ := json.Marshal(cmd)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if debug {
		log.Printf("soloist: > %s", data)
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// debug, with MYGOIFY_DEBUG set, logs what goes to Soloist and comes back,
// to line it up with what the listener heard.
var debug = os.Getenv("MYGOIFY_DEBUG") != ""

// summary is an event of Soloist's in a line: its type, status, the
// track's name and where it is.
func summary(data []byte) string {
	var ev event
	if json.Unmarshal(data, &ev) != nil {
		return string(data)
	}
	s := ev.Type
	if ev.Status != "" {
		s += " " + ev.Status
	}
	if ev.Item != nil {
		s += fmt.Sprintf(" %q", ev.Item.Decorations.Identity.Name)
	}
	if p := ev.Position; p != nil {
		s += fmt.Sprintf(" at %dms speed %g", p.PositionMs, p.Speed)
	}
	if ev.Message != "" {
		s += " " + ev.Message
	}
	return s
}

func (b *Backend) command(name string, args ...any) error {
	cmd := map[string]any{"command": name}
	for i := 0; i+1 < len(args); i += 2 {
		cmd[args[i].(string)] = args[i+1]
	}
	// Commands that play, to a Soloist that is not the device playing,
	// make it so; the others leave playback where it is, on the
	// listener's phone, say.
	b.mu.Lock()
	active := b.active
	b.mu.Unlock()
	if !active && plays[name] {
		if err := b.send(context.Background(), map[string]any{"command": "activate"}); err != nil {
			return err
		}
		// Soloist ignores what comes before it says it is the device
		// playing, half a second later: the app's play button did
		// nothing.
		if !b.waitActive(activateWait) {
			return errors.New("soloist: did not become the device playing")
		}
	}
	return b.send(context.Background(), cmd)
}

// activateWait is how long a command waits for Soloist to become the
// device playing.
const activateWait = 3 * time.Second

// waitActive waits up to d for Soloist to say it is the device playing,
// and reports whether it did.
func (b *Backend) waitActive(d time.Duration) bool {
	for deadline := time.Now().Add(d); ; {
		b.mu.Lock()
		active := b.active
		b.mu.Unlock()
		if active {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// plays are the commands that start playback.
var plays = map[string]bool{"play": true, "skip_next": true, "skip_prev": true}

func (b *Backend) Resume() error { return b.command("play") }
func (b *Backend) Pause() error  { return b.command("pause") }
func (b *Backend) Next() error   { return b.command("skip_next") }
func (b *Backend) Prev() error   { return b.command("skip_prev") }

func (b *Backend) Seek(t time.Duration) error {
	return b.command("seek", "position_ms", t.Milliseconds())
}

// SetVolume sets Soloist's volume. Before it connects, as the app sets
// the volume it kept as it starts, there is none to set: Soloist keeps
// its own.
func (b *Backend) SetVolume(v float64) error {
	err := b.command("set_volume", "volume", int(v*100+0.5))
	if errors.Is(err, errNotConnected) {
		return nil
	}
	return err
}

func (b *Backend) SetShuffle(on bool) error {
	return b.command("set_shuffle", "enabled", on)
}

func (b *Backend) SetRepeat(r playback.RepeatMode) error {
	if err := b.command("set_repeat_context", "enabled", r == playback.RepeatAll); err != nil {
		return err
	}
	return b.command("set_repeat_track", "enabled", r == playback.RepeatOne)
}

// AddToQueue queues t. Soloist takes it without being the device playing,
// so this leaves playback where it is.
func (b *Backend) AddToQueue(t music.Track) error {
	return b.command("add_to_queue", "uri", t.URI())
}

// maxURIs is the most tracks the Web API plays as a list.
const maxURIs = 100

// Play starts what req asks on this Soloist. A context from its start
// plays over the WebSocket; from a track of a context, or a list of
// tracks, as many around the one asked for as it takes, through the Web
// API. While Spotify limits the app's calls, those fail: played otherwise,
// as the track alone with those after it queued, they would change what
// the listener's other devices show playing, and leave the queue full.
func (b *Backend) Play(req playback.Request) error {
	if req.Context != "" && b.fromStart(req) {
		return b.command("play", "uri", req.Context)
	}
	body := map[string]any{}
	switch {
	case req.Context != "":
		body["context_uri"] = req.Context
		if req.Start >= 0 && req.Start < len(req.Tracks) {
			body["offset"] = map[string]any{"uri": req.Tracks[req.Start].URI()}
		}
	case len(req.Tracks) > 0:
		from := max(0, req.Start-maxURIs/2)
		to := min(len(req.Tracks), from+maxURIs)
		var uris []string
		for _, t := range req.Tracks[from:to] {
			uris = append(uris, t.URI())
		}
		body["uris"] = uris
		body["offset"] = map[string]any{"position": req.Start - from}
	default:
		return errors.New("soloist: nothing to play")
	}
	return b.playOnDevice(body)
}

// fromStart reports whether req plays its context as Soloist plays it
// from the start: from its first track the listener can play, not
// shuffled, as Soloist would pick another.
func (b *Backend) fromStart(req playback.Request) bool {
	b.mu.Lock()
	shuffle := b.options.Shuffle
	b.mu.Unlock()
	if shuffle {
		return false
	}
	if req.Start < 0 || req.Start >= len(req.Tracks) {
		return true
	}
	for _, t := range req.Tracks[:req.Start] {
		if t.Playable() {
			return false
		}
	}
	return true
}

// Jump plays the track at i of the queue: within the context it plays,
// through the Web API, else skipping to it.
func (b *Backend) Jump(i int) error {
	b.mu.Lock()
	st := b.state()
	ctxURI := b.ctxURI
	var source string
	if n := len(b.previous); i < n {
		source = b.previous[n-1-i].Source
	} else if i > n && i-n-1 < len(b.upcoming) {
		source = b.upcoming[i-n-1].Source
	}
	b.mu.Unlock()
	if i < 0 || i >= len(st.Queue) || i == st.At {
		return nil
	}
	if source == "context" && ctxURI != "" {
		return b.playOnDevice(map[string]any{"context_uri": ctxURI, "offset": map[string]any{"uri": st.Queue[i].URI()}})
	}
	step, n := "skip_next", i-st.At
	if n < 0 {
		step, n = "skip_prev", -n
	}
	for range n {
		if err := b.command(step); err != nil {
			return err
		}
	}
	return nil
}

// playOnDevice sends a play request for this Soloist to the Web API,
// finding its device again if the Web API no longer knows the one found.
func (b *Backend) playOnDevice(body map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for try := 0; ; try++ {
		dev, err := b.deviceID(ctx, try > 0)
		if err != nil {
			return err
		}
		err = b.cfg.API.Do(ctx, "PUT", "/me/player/play?device_id="+dev, body, nil)
		if spotify.IsNotFound(err) && try == 0 {
			continue
		}
		return err
	}
}

// deviceID returns this Soloist's ID in the Web API, as kept, or asking
// for the list of devices when none is, or when fresh.
func (b *Backend) deviceID(ctx context.Context, fresh bool) (string, error) {
	b.mu.Lock()
	dev := b.device
	b.mu.Unlock()
	if dev == "" && b.cfg.DeviceFile != "" {
		if data, err := os.ReadFile(b.cfg.DeviceFile); err == nil {
			dev = strings.TrimSpace(string(data))
		}
	}
	if dev != "" && !fresh {
		return dev, nil
	}
	var res struct {
		Devices []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"devices"`
	}
	if err := b.cfg.API.Do(ctx, "GET", "/me/player/devices", nil, &res); err != nil {
		return "", err
	}
	for _, d := range res.Devices {
		if d.Name == b.cfg.DeviceName {
			b.mu.Lock()
			b.device = d.ID
			b.mu.Unlock()
			if f := b.cfg.DeviceFile; f != "" {
				os.MkdirAll(filepath.Dir(f), 0o700)
				os.WriteFile(f, []byte(d.ID+"\n"), 0o600)
			}
			return d.ID, nil
		}
	}
	return "", fmt.Errorf("soloist: Spotify lists no device named %q: is Soloist running and paired?", b.cfg.DeviceName)
}
