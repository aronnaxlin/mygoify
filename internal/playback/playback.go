// Package playback is what plays and what the app shows of it. Player
// holds where playback is, on the main thread, and passes the listener's
// commands to a Backend, which plays: Soloist, or Fake in tests and in
// development.
package playback

import (
	"time"

	"github.com/egoist/mygo"

	"mygoify/internal/music"
)

type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatAll
	RepeatOne
)

// Bands is how many bands Levels splits what plays into.
const Bands = 24

// Request is what to play: a context, as an album or a playlist, from the
// track at Start of those it lists; or, with no context, Tracks alone,
// as a search lists them. Tracks are the context's as the page shows
// them, to show until the backend says what it plays.
type Request struct {
	Context string
	Tracks  []music.Track
	Start   int
}

// State is where playback is, as the backend reports it.
type State struct {
	Queue []music.Track // in the order they play, what played first
	At    int           // the track playing in Queue, -1 for none
	// Playing is what the listener wants: false while paused, true while
	// a track loads to play.
	Playing bool
	Loading bool
	// Position is where the track was at Since; playing, it moves on from
	// there.
	Position time.Duration
	Since    time.Time
	Shuffle  bool
	Repeat   RepeatMode
	Err      string
}

// Backend plays. Its methods may block, and are called one at a time, off
// the main thread; it reports where playback is through the function it
// was given to Watch, on any goroutine.
type Backend interface {
	Watch(func(State))
	Play(Request) error
	Resume() error
	Pause() error
	Next() error
	Prev() error
	// Jump plays the track at i in the queue.
	Jump(i int) error
	Seek(time.Duration) error
	SetVolume(float64) error
	SetShuffle(bool) error
	SetRepeat(RepeatMode) error
	// AddToQueue queues t after the tracks queued before it, ahead of
	// the rest of the context.
	AddToQueue(t music.Track) error
}

// Player is where playback is, as the app shows it. Its methods run on the
// main thread: they show what they ask for at once, and the backend's
// state replaces it as it reports.
type Player struct {
	post func(func())
	b    Backend
	cmds chan func() error

	// Explain, if set, says why a command failed as the listener reads
	// it, in place of msg; it is called off the main thread.
	Explain func(err error, msg string) string

	// Changed, if set, hears that what plays, where it is or whether it
	// plays changed.
	Changed func()
	// Artwork, if set, fetches a cover, to show the system what plays.
	Artwork func(url string) ([]byte, error)

	st   State
	held bool // a test holds the position where it is
	// expect is the track asked to play, until the backend says it plays
	// it: what it says before is of the track before.
	expect   string
	expectAt time.Time
	volume   float64
	shown    string // the track the system was last shown
}

// New returns a player of what b plays, which follows it once asked to
// (Follow).
// post runs a function on the main thread.
func New(post func(func()), b Backend) *Player {
	p := &Player{post: post, b: b, st: State{At: -1}, volume: 0.8, cmds: make(chan func() error, 64)}
	go p.run()
	// The media keys, and the controls of the system's Now Playing.
	mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
		p.post(func() {
			switch cmd {
			case mygo.MediaPlay, mygo.MediaPause:
				if (cmd == mygo.MediaPlay) != p.st.Playing {
					p.TogglePlay()
				}
			case mygo.MediaNext:
				p.Next()
			case mygo.MediaPrevious:
				p.Prev()
			case mygo.MediaSeek:
				p.Seek(pos)
			}
		})
	})
	return p
}

// Follow follows what the backend plays. The app calls it once post
// brings functions to its window: what the backend says as it first
// connects, before, was lost, and the player bar showed nothing while
// music played on.
func (p *Player) Follow() {
	p.b.Watch(func(st State) { p.post(func() { p.apply(st) }) })
}

// run sends the commands to the backend in the order they were given.
func (p *Player) run() {
	for cmd := range p.cmds {
		if err := cmd(); err != nil {
			msg := "播放失败，请稍后再试"
			if p.Explain != nil {
				msg = p.Explain(err, msg)
			}
			p.post(func() {
				p.st.Loading = false
				p.st.Err = msg
				p.sync()
			})
		}
	}
}

func (p *Player) send(cmd func() error) {
	select {
	case p.cmds <- cmd:
	default:
		// The backend is far behind: drop what it can't keep up with
		// rather than block the main thread.
	}
}

// expectFor is how long the player waits for the track asked to play
// before it shows what the backend says plays all the same.
const expectFor = 5 * time.Second

func (p *Player) apply(st State) {
	// Soloist says where the track before was once more before it plays
	// the one asked for: the player bar went back to it a moment, and
	// seemed to start the new one twice.
	if p.expect != "" {
		var id string
		if st.At >= 0 && st.At < len(st.Queue) {
			id = st.Queue[st.At].ID
		}
		if id != p.expect && time.Since(p.expectAt) < expectFor {
			return
		}
		p.expect = ""
	}
	p.st = st
	p.held = false
	p.sync()
}

// sync shows the system, and Changed, where playback is.
func (p *Player) sync() {
	if p.Changed != nil {
		defer p.Changed()
	}
	t := p.Current()
	if t == nil {
		p.shown = ""
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPaused
	if p.st.Playing {
		state = mygo.PlaybackPlaying
	}
	if t.ID == p.shown {
		mygo.NowPlaying.SetPlayback(state, p.Position())
		return
	}
	p.shown = t.ID
	info := mygo.NowPlayingInfo{
		Title:    t.Name,
		Artist:   t.ArtistNames(),
		Album:    t.Album.Name,
		Duration: t.Length(),
		Position: p.Position(),
		State:    state,
		Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
	}
	mygo.NowPlaying.Set(info)
	if url := t.Album.Images.URL(600); url != "" && p.Artwork != nil {
		id := t.ID
		go func() {
			cover, err := p.Artwork(url)
			if err != nil {
				return
			}
			p.post(func() {
				if p.shown == id {
					info.Artwork, info.Position = cover, p.Position()
					mygo.NowPlaying.Set(info)
				}
			})
		}()
	}
}

// PlayQueue plays tracks from the one at start: within context, if set,
// the album or playlist they are.
//
// A context may come without tracks, as a playlist Spotify lists none of:
// nothing shows playing until the backend says what does.
func (p *Player) PlayQueue(context string, tracks []music.Track, start int) {
	if len(tracks) == 0 && context == "" {
		return
	}
	start = min(max(start, 0), len(tracks)-1)
	if len(tracks) == 0 {
		start = -1
	}
	p.st = State{Queue: tracks, At: start, Playing: true, Loading: true, Since: time.Now(), Shuffle: p.st.Shuffle, Repeat: p.st.Repeat}
	p.expect, p.expectAt = "", time.Now()
	if start >= 0 {
		p.expect = tracks[start].ID
	}
	p.sync()
	req := Request{Context: context, Tracks: tracks, Start: start}
	p.send(func() error { return p.b.Play(req) })
}

func (p *Player) ToggleShuffle() {
	p.st.Shuffle = !p.st.Shuffle
	on := p.st.Shuffle
	p.send(func() error { return p.b.SetShuffle(on) })
}

func (p *Player) CycleRepeat() {
	p.st.Repeat = (p.st.Repeat + 1) % 3
	r := p.st.Repeat
	p.send(func() error { return p.b.SetRepeat(r) })
}

// AddToQueue queues t to play after the tracks queued before it, ahead of
// the rest of what plays. done hears, on the main thread, whether the
// backend did: the queue shows it as the backend reports it, and failing,
// it isn't the track playing that failed.
func (p *Player) AddToQueue(t music.Track, done func(error)) {
	p.send(func() error {
		err := p.b.AddToQueue(t)
		p.post(func() { done(err) })
		return nil
	})
}

// Current returns the track playing or loading, or nil.
func (p *Player) Current() *music.Track {
	if p.st.At < 0 || p.st.At >= len(p.st.Queue) {
		return nil
	}
	return &p.st.Queue[p.st.At]
}

// Queue returns the tracks queued, in the order they play.
func (p *Player) Queue() []music.Track { return p.st.Queue }

// PlayOrder returns the queue's indices in play order, which is theirs,
// and where the current track is among them, -1 for none.
func (p *Player) PlayOrder() ([]int, int) {
	order := make([]int, len(p.st.Queue))
	for i := range order {
		order[i] = i
	}
	return order, p.st.At
}

// Playing reports what the listener wants: false while paused.
func (p *Player) Playing() bool { return p.st.Playing }

// Loading reports whether the current track is still loading.
func (p *Player) Loading() bool { return p.st.Loading }

// Ready reports whether the current track has loaded, to play.
func (p *Player) Ready() bool { return p.Current() != nil && !p.st.Loading }

// Err returns why the current track did not play, or "".
func (p *Player) Err() string { return p.st.Err }

func (p *Player) Shuffle() bool      { return p.st.Shuffle }
func (p *Player) Repeat() RepeatMode { return p.st.Repeat }

// move shows the queue's track at, until the backend says what plays.
func (p *Player) move(at int) {
	p.st.At, p.st.Position, p.st.Since, p.st.Err = at, 0, time.Now(), ""
	p.st.Playing, p.st.Loading = true, true
	p.sync()
}

func (p *Player) Next() {
	if n := len(p.st.Queue); n > 0 {
		p.move((p.st.At + 1) % n)
		p.send(p.b.Next)
	}
}

// Prev plays the track before this one, however far this one has played:
// once a few seconds in, going back to its start only kept the listener
// pressing twice.
func (p *Player) Prev() {
	if n := len(p.st.Queue); n > 0 {
		p.move((p.st.At - 1 + n) % n)
		p.send(p.b.Prev)
	}
}

// Jump plays the queue's track i.
func (p *Player) Jump(i int) {
	if i >= 0 && i < len(p.st.Queue) {
		p.move(i)
		p.send(func() error { return p.b.Jump(i) })
	}
}

// TogglePlay pauses or plays.
func (p *Player) TogglePlay() {
	if p.Current() == nil {
		return
	}
	pos := p.Position()
	p.st.Playing = !p.st.Playing
	p.st.Position, p.st.Since = pos, time.Now()
	p.sync()
	if p.st.Playing {
		p.send(p.b.Resume)
	} else {
		p.send(p.b.Pause)
	}
}

// Seek moves to t.
func (p *Player) Seek(t time.Duration) {
	p.st.Position, p.st.Since = t, time.Now()
	p.sync()
	p.send(func() error { return p.b.Seek(t) })
}

// Volume returns the volume, from 0 to 1.
func (p *Player) Volume() float64 { return p.volume }

func (p *Player) SetVolume(v float64) {
	p.volume = v
	p.send(func() error { return p.b.SetVolume(v) })
}

// Position returns where the track playing is, moving on from where the
// backend last said while it plays.
func (p *Player) Position() time.Duration {
	// Nothing playing is nowhere: Soloist, gone idle, still says where
	// the track before was.
	if p.Current() == nil {
		return 0
	}
	pos := p.st.Position
	if p.st.Playing && !p.st.Loading && !p.held && !p.st.Since.IsZero() {
		pos += time.Since(p.st.Since)
	}
	if d := p.Duration(); d > 0 {
		pos = min(pos, d)
	}
	return max(pos, 0)
}

func (p *Player) Duration() time.Duration {
	if t := p.Current(); t != nil {
		return t.Length()
	}
	return 0
}

// Levels returns the spectrum of what plays now, and false when there is
// none to show. Spotify's players keep their audio to themselves: there
// is none.
func (p *Player) Levels() ([Bands]float32, bool) { return [Bands]float32{}, false }
