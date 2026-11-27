package agent

import (
	"testing"
	"time"

	"mygoify/internal/music"
	"mygoify/internal/playback"
)

// ctl records what the MPRIS player asks.
type ctl struct {
	seeked []time.Duration
	next   int
}

func (c *ctl) Resume() error                       { return nil }
func (c *ctl) Pause() error                        { return nil }
func (c *ctl) Next() error                         { c.next++; return nil }
func (c *ctl) Prev() error                         { return nil }
func (c *ctl) Seek(t time.Duration) error          { c.seeked = append(c.seeked, t); return nil }
func (c *ctl) SetVolume(float64) error             { return nil }
func (c *ctl) SetShuffle(bool) error               { return nil }
func (c *ctl) SetRepeat(playback.RepeatMode) error { return nil }

func playing(pos time.Duration, since time.Time) playback.State {
	return playback.State{
		Queue: []music.Track{{ID: "a", Duration: 200000}}, At: 0,
		Playing: true, Position: pos, Since: since,
	}
}

// Where a playing track is moves on from where Soloist last said, and stops
// at its end; a paused one stays.
func TestPosition(t *testing.T) {
	now := time.Now()
	st := playing(10*time.Second, now.Add(-5*time.Second))
	if got := position(st, now); got != 15*time.Second {
		t.Errorf("playing: %v", got)
	}
	if got := position(playing(190*time.Second, now.Add(-time.Minute)), now); got != 200*time.Second {
		t.Errorf("past the end: %v", got)
	}
	st.Playing = false
	if got := position(st, now); got != 10*time.Second {
		t.Errorf("paused: %v", got)
	}
}

// Seek moves by an offset from where the track is, not from its start; past
// the end it plays the next track, and before the start it goes to the start,
// as MPRIS says.
func TestSeekBy(t *testing.T) {
	c := &ctl{}
	m := &mpris{ctl: c, st: playing(60*time.Second, time.Now())}
	p := player{m}
	p.SeekBy((10 * time.Second).Microseconds())
	p.SeekBy((-90 * time.Second).Microseconds())
	if len(c.seeked) != 2 || c.seeked[0] < 70*time.Second || c.seeked[0] > 71*time.Second || c.seeked[1] != 0 {
		t.Fatalf("seeked to %v", c.seeked)
	}
	p.SeekBy((200 * time.Second).Microseconds())
	if c.next != 1 || len(c.seeked) != 2 {
		t.Errorf("past the end: next %d, seeked %v", c.next, c.seeked)
	}
}

// SetPosition moves only the track it names: one that ended meanwhile is
// left alone.
func TestSetPosition(t *testing.T) {
	c := &ctl{}
	m := &mpris{ctl: c, st: playing(0, time.Now())}
	p := player{m}
	p.SetPosition(trackPath("b"), (30 * time.Second).Microseconds())
	p.SetPosition(trackPath("a"), (30 * time.Second).Microseconds())
	if len(c.seeked) != 1 || c.seeked[0] != 30*time.Second {
		t.Errorf("seeked to %v", c.seeked)
	}
}
