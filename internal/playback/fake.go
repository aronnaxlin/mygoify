package playback

import (
	"errors"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"mygoify/internal/music"
)

// Fake plays nothing, but keeps time as if it did: tracks end, and the
// next plays, as the queue's order, shuffle and repeat say.
type Fake struct {
	mu     sync.Mutex
	states chan State // what emit reports, in order, for Watch's function
	st     State
	given  []music.Track // the queue in the order Play gave it
	queued int           // how many of the tracks after the one playing were queued
	timer  *time.Timer
	volume float64
}

func NewFake() *Fake { return &Fake{st: State{At: -1}} }

// Watch reports each state to fn, on a goroutine of its own, in the order
// they came: one reported later is newer.
func (f *Fake) Watch(fn func(State)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states = make(chan State, 64)
	go func(states chan State) {
		for st := range states {
			fn(st)
		}
	}(f.states)
}

// emit reports the state, and sets the timer for the track's end. f.mu is
// held.
func (f *Fake) emit() {
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	if f.st.Playing && f.st.At >= 0 {
		left := f.st.Queue[f.st.At].Length() - f.position()
		f.timer = time.AfterFunc(max(left, 0), f.ended)
	}
	st := f.st
	st.Queue = slices.Clone(f.st.Queue)
	if f.states != nil {
		f.states <- st
	}
}

func (f *Fake) position() time.Duration {
	if !f.st.Playing {
		return f.st.Position
	}
	return f.st.Position + time.Since(f.st.Since)
}

func (f *Fake) at(pos time.Duration) {
	f.st.Position, f.st.Since = pos, time.Now()
}

func (f *Fake) ended() {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.st.Repeat == RepeatOne:
		f.at(0)
	case f.st.At+1 < len(f.st.Queue):
		f.st.At++
		f.queued = max(f.queued-1, 0)
		f.at(0)
	case f.st.Repeat == RepeatAll:
		f.st.At = 0
		f.at(0)
	default:
		f.at(f.st.Queue[f.st.At].Length())
		f.st.Playing = false
	}
	f.emit()
}

var errNothing = errors.New("playback: nothing to play")

func (f *Fake) Play(req Request) error {
	if len(req.Tracks) == 0 {
		// A context alone: Fake knows no albums or playlists of its own.
		return errNothing
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.Queue = slices.Clone(req.Tracks)
	f.given = slices.Clone(req.Tracks)
	f.queued = 0
	f.st.At = min(max(req.Start, 0), len(req.Tracks)-1)
	if f.st.Shuffle {
		f.shuffle()
	}
	f.st.Playing, f.st.Loading, f.st.Err = true, false, ""
	f.at(0)
	f.emit()
	return nil
}

// shuffle puts the tracks after the one playing in a random order, as
// Spotify does: what played stays, and so do those queued. f.mu is held.
func (f *Fake) shuffle() {
	rest := f.st.Queue[f.st.At+1+f.queued:]
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
}

func (f *Fake) Resume() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.st.At < 0 || f.st.Playing {
		return nil
	}
	if f.st.Position >= f.st.Queue[f.st.At].Length() {
		f.st.Position = 0
	}
	f.at(f.st.Position)
	f.st.Playing = true
	f.emit()
	return nil
}

func (f *Fake) Pause() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.st.Playing {
		return nil
	}
	f.at(f.position())
	f.st.Playing = false
	f.emit()
	return nil
}

func (f *Fake) jump(at int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n := len(f.st.Queue); n == 0 {
		return errNothing
	} else {
		at = (at%n + n) % n
	}
	// Moving on past queued tracks plays them, or skips them.
	if d := at - f.st.At; d > 0 {
		f.queued = max(f.queued-d, 0)
	}
	f.st.At = at
	f.st.Playing = true
	f.at(0)
	f.emit()
	return nil
}

func (f *Fake) Next() error      { return f.jump(f.current() + 1) }
func (f *Fake) Prev() error      { return f.jump(f.current() - 1) }
func (f *Fake) Jump(i int) error { return f.jump(i) }

func (f *Fake) current() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st.At
}

func (f *Fake) Seek(t time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.at(t)
	f.emit()
	return nil
}

func (f *Fake) SetVolume(v float64) error {
	f.mu.Lock()
	f.volume = v
	f.mu.Unlock()
	return nil
}

// SetShuffle shuffles the tracks after the one playing, or puts them back
// in the order Play gave them.
func (f *Fake) SetShuffle(on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.Shuffle = on
	if f.st.At >= 0 {
		if on {
			f.shuffle()
		} else {
			id := f.st.Queue[f.st.At].ID
			f.st.Queue = slices.Clone(f.given)
			f.queued = 0
			f.st.At = max(slices.IndexFunc(f.given, func(t music.Track) bool { return t.ID == id }), 0)
		}
	}
	f.emit()
	return nil
}

func (f *Fake) SetRepeat(r RepeatMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.Repeat = r
	f.emit()
	return nil
}

// AddToQueue puts t after the tracks queued before it, ahead of the rest
// of the queue, as Spotify does.
func (f *Fake) AddToQueue(t music.Track) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.st.At < 0 {
		return errNothing
	}
	f.st.Queue = slices.Insert(f.st.Queue, f.st.At+1+f.queued, t)
	f.queued++
	f.emit()
	return nil
}
