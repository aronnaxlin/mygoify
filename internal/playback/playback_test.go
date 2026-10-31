package playback

import (
	"sync/atomic"
	"testing"
	"time"

	"mygoify/internal/music"
)

func tracks(n int, length time.Duration) []music.Track {
	var out []music.Track
	for i := range n {
		out = append(out, music.Track{ID: string(rune('a' + i)), Name: string(rune('A' + i)), Duration: length.Milliseconds()})
	}
	return out
}

// states collects what a fake reports, waiting for the one asked for.
func states(f *Fake) func(t *testing.T, ok func(State) bool) State {
	ch := make(chan State, 64)
	f.Watch(func(st State) { ch <- st })
	return func(t *testing.T, ok func(State) bool) State {
		t.Helper()
		timeout := time.After(2 * time.Second)
		for {
			select {
			case st := <-ch:
				if ok(st) {
					return st
				}
			case <-timeout:
				t.Fatal("no such state")
			}
		}
	}
}

// A track that ends plays the next, and the last stops, unless repeat
// goes round.
func TestFakeEnds(t *testing.T) {
	f := NewFake()
	next := states(f)
	f.Play(Request{Tracks: tracks(2, 30*time.Millisecond), Start: 0})
	next(t, func(st State) bool { return st.At == 1 && st.Playing })
	next(t, func(st State) bool { return st.At == 1 && !st.Playing })

	f.SetRepeat(RepeatAll)
	f.Play(Request{Tracks: tracks(2, 30*time.Millisecond), Start: 1})
	next(t, func(st State) bool { return st.At == 0 && st.Playing })
}

// Shuffling leaves what played and the track playing where they are, and
// shuffling off puts the queue back as it was given.
func TestFakeShuffle(t *testing.T) {
	f := NewFake()
	next := states(f)
	given := tracks(12, time.Minute)
	f.Play(Request{Tracks: given, Start: 3})
	f.SetShuffle(true)
	st := next(t, func(st State) bool { return st.Shuffle })
	for i := 0; i <= 3; i++ {
		if st.Queue[i].ID != given[i].ID {
			t.Fatalf("shuffled what played: %v", st.Queue[i].Name)
		}
	}
	f.Next()
	f.SetShuffle(false)
	st = next(t, func(st State) bool { return !st.Shuffle })
	for i, tr := range st.Queue {
		if tr.ID != given[i].ID {
			t.Fatalf("the order is not back: %d is %s", i, tr.Name)
		}
	}
	if st.Queue[st.At].ID == given[3].ID {
		t.Fatal("still on the track from before Next")
	}
}

// Paused, the position stays; playing, it moves on from where the backend
// last put it.
func TestPlayerPosition(t *testing.T) {
	p := New(func(func()) {}, NewFake())
	p.Cue(tracks(1, time.Minute))
	p.Seek(10 * time.Second)
	if pos := p.Position(); pos != 10*time.Second {
		t.Fatalf("paused at %v", pos)
	}
	p.TogglePlay()
	time.Sleep(30 * time.Millisecond)
	if pos := p.Position(); pos < 10*time.Second+20*time.Millisecond {
		t.Fatalf("playing, still at %v", pos)
	}
	p.TogglePlay()
	at := p.Position()
	time.Sleep(20 * time.Millisecond)
	if pos := p.Position(); pos != at {
		t.Fatalf("paused, moved from %v to %v", at, pos)
	}
}

// Next and Prev show the track they go to at once, going round the queue.
func TestPlayerNextPrev(t *testing.T) {
	p := New(func(func()) {}, NewFake())
	p.Cue(tracks(3, time.Minute))
	p.Prev()
	if p.Current().Name != "C" {
		t.Fatalf("Prev from the first went to %s", p.Current().Name)
	}
	p.Next()
	p.Next()
	if p.Current().Name != "B" || !p.Playing() {
		t.Fatalf("went to %s, playing %v", p.Current().Name, p.Playing())
	}
}

// With nothing playing the position is 0, whatever the backend says: the
// player bar showed where Soloist's last track had got to.
func TestPlayerPositionWithNothing(t *testing.T) {
	p := New(func(func()) {}, NewFake())
	p.apply(State{At: -1, Position: 4 * time.Minute})
	if pos := p.Position(); pos != 0 {
		t.Fatalf("at %v with nothing playing", pos)
	}
}

// Asked to play a track, the player shows it until the backend says it
// plays it, not what the backend says of the track before: the player bar
// went back to it a moment, and seemed to start the new one twice.
func TestPlayerWaitsForTheTrackAsked(t *testing.T) {
	p := New(func(func()) {}, NewFake())
	old, asked := tracks(1, time.Minute), tracks(3, time.Minute)
	p.PlayQueue("", asked, 2)
	p.apply(State{Queue: old, At: 0, Playing: true, Position: 40 * time.Second, Since: time.Now()})
	if p.Current().ID != asked[2].ID {
		t.Fatalf("went back to %s", p.Current().Name)
	}
	p.apply(State{Queue: asked, At: 2, Playing: true, Since: time.Now()})
	p.apply(State{Queue: asked, At: 1, Playing: true, Since: time.Now()})
	if p.Current().ID != asked[1].ID {
		t.Fatalf("the backend moving on, still on %s", p.Current().Name)
	}
}

// eager reports the state it has as soon as it is watched, as Soloist
// does once connected.
type eager struct {
	*Fake
	st State
}

func (e eager) Watch(fn func(State)) { go fn(e.st) }

// What the backend says as the player first follows it shows: it said it
// before the app's window was there to take it, and the player bar showed
// nothing while music played on.
func TestPlayerFollowsOnceAsked(t *testing.T) {
	var ready atomic.Bool
	applied := make(chan func(), 1)
	post := func(fn func()) {
		if ready.Load() {
			applied <- fn
		}
	}
	playing := tracks(1, time.Minute)
	p := New(post, eager{NewFake(), State{Queue: playing, At: 0, Playing: true}})
	ready.Store(true) // the window is there
	p.Follow()
	select {
	case fn := <-applied:
		fn()
	case <-time.After(time.Second):
		t.Fatal("nothing reported")
	}
	if p.Current() == nil || p.Current().ID != playing[0].ID {
		t.Fatal("the track playing doesn't show")
	}
}
