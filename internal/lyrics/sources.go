package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// missAgain is how long a track with no lyrics waits before it is
	// asked for again: someone may have added them.
	missAgain = 7 * 24 * time.Hour
	// grace is how long what a source found is shown after: the sources
	// still asked have that long to better it, with word times or a
	// translation. A song without a translation waits no longer, as
	// LRCLIB has none to give and may take a minute to say it is busy.
	grace = 1500 * time.Millisecond
)

// A Source finds a track's lyrics, somewhere.
type Source interface {
	// Name is how the source is known in the cache of its answers.
	Name() string
	// Find returns the lyrics of q, none if the source has none, and an
	// error only if it couldn't be asked: that is not kept as the track
	// having no lyrics, and asking again may find them.
	Find(ctx context.Context, q Query) (Found, error)
}

// Found is what a source has of a track's lyrics: its lines, word-timed in
// YRC where it has them, else timed in LRC, and the translation of the
// lines.
type Found struct {
	LRC, Translation, YRC string
}

// Lines returns the lines to show of what was found, or none.
func (f Found) Lines() []Line {
	return Build(f.LRC, f.Translation, f.YRC)
}

// Sources asks its sources one after another for a track's lyrics: the
// first with word-timed lines wins, or else the first with timed ones,
// with the translation of a source that has one. What it finds, and what
// it doesn't, it keeps in Cache, by track.
type Sources struct {
	All   []Source
	Cache string // the directory of what was found; none if empty

	now   func() time.Time
	after func(time.Duration) <-chan time.Time // how long a wait ends, time.After unless a test sets it
}

// A pick is a track a source may have the lyrics of, as it compares to
// the query.
type pick struct {
	Length    time.Duration
	Album     string
	HasLyrics bool
}

// nearestOf returns the index of the pick nearest q in length, within
// near, and of q's album if several are as near: soundtracks and singles
// carry the same song. It returns -1 if none is.
func nearestOf(picks []pick, q Query, near time.Duration) int {
	best, gap := -1, near+1
	for i, p := range picks {
		if !p.HasLyrics {
			continue
		}
		g := p.Length - q.Duration
		if g < 0 {
			g = -g
		}
		g = g.Round(500 * time.Millisecond)
		if g > near {
			continue
		}
		// Of as near ones, the album's: only when asked with one, else
		// every pick without an album would win over the one before it.
		if g < gap || g == gap && q.Album != "" && strings.EqualFold(p.Album, q.Album) && !strings.EqualFold(picks[best].Album, q.Album) {
			best, gap = i, g
		}
	}
	return best
}

// kept is what the cache keeps of a track: its lyrics, empty when there
// are none, when they were asked for, the source that had them, and the
// sources asked then, so that turning one on asks again.
type kept struct {
	At          time.Time
	LRC         string
	Translation string
	YRC         string
	Source      string // a source's name; empty for lrclib, as the oldest kept
	Asked       string // their names, comma-separated; empty for lrclib
}

// Find returns the timed lines of q, or none if no source has them, and an
// error only if one couldn't be asked: that isn't kept, and asking again
// may find them.
func (s *Sources) Find(ctx context.Context, q Query) ([]Line, error) {
	// A track not named yet, as Soloist's first of a context another
	// device plays, has nothing to look for, and the sources refuse it. It
	// isn't kept, as the name may come.
	if strings.TrimSpace(q.Name) == "" {
		return nil, nil
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	asked := s.names()
	if k, ok := s.load(q.ID); ok {
		// ponytail: lyrics found once are kept for good, even if a source
		// turned on later has word times of them; ask again when a source
		// that has them is turned on, if that turns out to matter.
		if k.LRC != "" || k.YRC != "" {
			return k.found().Lines(), nil
		}
		// Asked before a source was turned on, or off, the track may be
		// found now: it is asked again.
		if now().Sub(k.At) < missAgain && k.asked() == asked {
			return nil, nil
		}
	}

	// Asked at once: a source that has lyrics in a third of a second
	// isn't waited for while another times out for a minute.
	type answer struct {
		from  string
		found Found
		err   error
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	answers := make(chan answer, len(s.All))
	for _, src := range s.All {
		go func() {
			f, err := src.Find(ctx, q)
			answers <- answer{src.Name(), f, err}
		}()
	}
	// keep stores what was found, and returns its lines.
	keep := func(f Found, from string) ([]Line, error) {
		s.store(q.ID, kept{At: now(), LRC: f.LRC, Translation: f.Translation, YRC: f.YRC, Source: from, Asked: asked})
		return f.Lines(), nil
	}

	after := s.after
	if after == nil {
		after = time.After
	}
	var best Found
	var translation, from string
	var errs []error
	var betterWait <-chan time.Time // ends the wait for better, once there is something to show
	for range s.All {
		var a answer
		if betterWait == nil {
			a = <-answers
		} else {
			select {
			case a = <-answers:
			case <-betterWait:
				// Those still asked may have better, but what is there
				// shows now.
				cancel()
				return keep(best, from)
			}
		}
		if a.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.from, a.err))
			continue
		}
		// A source may have the translation of lines another has: of
		// whichever source has one of its own, the first, else the first
		// translation of any.
		if a.found.Translation != "" && translation == "" && (a.found.LRC != "" || a.found.YRC != "") {
			translation = a.found.Translation
		}
		switch {
		case a.found.YRC != "" && best.YRC == "":
			best, from = a.found, a.from
		case a.found.LRC != "" && best.LRC == "" && best.YRC == "":
			best, from = a.found, a.from
		}
		if best.Translation == "" {
			best.Translation = translation
		}
		// Word times and their translation can't be bettered: the sources
		// still asked aren't waited for.
		if best.YRC != "" && best.Translation != "" {
			cancel()
			return keep(best, from)
		}
		if betterWait == nil && (best.LRC != "" || best.YRC != "") {
			betterWait = after(grace)
		}
	}
	if best.Translation == "" {
		best.Translation = translation
	}
	if best.LRC != "" || best.YRC != "" {
		return keep(best, from)
	}
	// A source that couldn't be asked may have them: not kept as none.
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	s.store(q.ID, kept{At: now(), Asked: asked})
	return nil, nil
}

// names is the sources asked, as the cache keeps it.
func (s *Sources) names() string {
	names := make([]string, len(s.All))
	for i, src := range s.All {
		names[i] = src.Name()
	}
	return strings.Join(names, ",")
}

func (k kept) found() Found {
	return Found{LRC: k.LRC, Translation: k.Translation, YRC: k.YRC}
}

// asked is the sources asked when kept, the only one there was before
// there were others.
func (k kept) asked() string {
	if k.Asked == "" {
		return "lrclib"
	}
	return k.Asked
}

func (s *Sources) file(id string) string {
	if s.Cache == "" || id == "" || strings.ContainsAny(id, `/\.`) {
		return ""
	}
	return filepath.Join(s.Cache, id+".json")
}

func (s *Sources) load(id string) (kept, bool) {
	var k kept
	f := s.file(id)
	if f == "" {
		return k, false
	}
	b, err := os.ReadFile(f)
	if err != nil || json.Unmarshal(b, &k) != nil {
		return k, false
	}
	return k, true
}

func (s *Sources) store(id string, k kept) {
	f := s.file(id)
	if f == "" {
		return
	}
	b, err := json.Marshal(k)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(f), 0o700)
	// Written whole, then renamed: a crash leaves the old file.
	tmp := f + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, f)
	}
}
