package playback

import (
	"time"

	"mygoify/internal/music"
)

// These set a player up as a backend would leave it, without one playing,
// for the tests of the views that show it.

// Cue makes tracks the queue, in order, with the first current, paused.
func (p *Player) Cue(tracks []music.Track) {
	p.st = State{Queue: tracks, At: 0}
	p.held = false
}

// Hold makes the player play at pos, staying there.
func (p *Player) Hold(pos time.Duration) {
	p.st.Position, p.st.Since = pos, time.Now()
	p.st.Playing, p.st.Loading = true, false
	p.held = true
}

// Start makes the player play from where it is, the position moving on.
func (p *Player) Start() {
	p.st.Playing, p.st.Loading = true, false
	p.st.Since = time.Now()
	p.held = false
}
