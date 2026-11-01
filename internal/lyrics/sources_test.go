package lyrics

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSource is a source that answers what a test sets, counting what it
// is asked.
type fakeSource struct {
	name  string
	found Found
	err   error
	asked int
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Find(context.Context, Query) (Found, error) {
	f.asked++
	return f.found, f.err
}

// sourceFunc is a source made of a function, for tests.
type sourceFunc struct {
	name string
	find func(ctx context.Context, q Query) (Found, error)
}

func (f sourceFunc) Name() string { return f.name }

func (f sourceFunc) Find(ctx context.Context, q Query) (Found, error) { return f.find(ctx, q) }

// found asks a source for q's lyrics, as lines.
func found(src Source, q Query) ([]Line, error) {
	f, err := src.Find(context.Background(), q)
	return f.Lines(), err
}

// A source's word times and their translation end the search at once, and
// the sources still asked are cancelled. Before, the sources were asked
// one after another, and lyrics NetEase had in a third of a second showed
// once LRCLIB had timed out a minute later.
func TestSourcesReturnWordTimesAtOnce(t *testing.T) {
	words := sourceFunc{name: "words", find: func(context.Context, Query) (Found, error) {
		return Found{YRC: "[1000,500](1000,500,0)春(1500,500,0)日影\n", Translation: "[00:01.00]春日之影\n"}, nil
	}}
	cancelled := make(chan struct{})
	slow := sourceFunc{name: "slow", find: func(ctx context.Context, _ Query) (Found, error) {
		<-ctx.Done()
		close(cancelled)
		return Found{}, ctx.Err()
	}}
	s := &Sources{All: []Source{words, slow}}
	done := make(chan []Line, 1)
	go func() {
		lines, err := s.Find(context.Background(), spring)
		if err != nil {
			t.Error(err)
		}
		done <- lines
	}()
	select {
	case lines := <-done:
		if len(lines) != 1 || !lines[0].Timed || lines[0].Translation != "春日之影" {
			t.Fatalf("found %+v", lines)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waited for a source with nothing better to give")
	}
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the source still asked wasn't cancelled")
	}
}

// Word times a source found show once the short wait is over, rather than
// when the slowest source times out: a Chinese song without a translation
// used to wait for LRCLIB to give up a minute later, though LRCLIB has no
// translations at all.
func TestSourcesShowWordTimesAfterTheWait(t *testing.T) {
	words := sourceFunc{name: "words", find: func(context.Context, Query) (Found, error) {
		return Found{YRC: "[1000,500](1000,500,0)春(1500,500,0)日影\n"}, nil
	}}
	slow := sourceFunc{name: "slow", find: func(ctx context.Context, _ Query) (Found, error) {
		<-ctx.Done()
		return Found{}, ctx.Err()
	}}
	over := make(chan time.Time)
	s := &Sources{All: []Source{words, slow}, after: func(time.Duration) <-chan time.Time { return over }}
	done := make(chan []Line, 1)
	go func() {
		lines, err := s.Find(context.Background(), spring)
		if err != nil {
			t.Error(err)
		}
		done <- lines
	}()
	select {
	case over <- time.Now(): // the wait is over
	case <-time.After(5 * time.Second):
		t.Fatal("returned before the wait for better was over")
	}
	select {
	case lines := <-done:
		if len(lines) != 1 || !lines[0].Timed {
			t.Fatalf("found %+v", lines)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waited for a source that can't better it")
	}
}

// Timed lines wait the short while for another source's word times: the
// better ones show, not the first to answer.
func TestSourcesWaitForWordTimes(t *testing.T) {
	lines := sourceFunc{name: "lines", find: func(context.Context, Query) (Found, error) {
		return Found{LRC: "[00:01.00]春日影\n"}, nil
	}}
	release := make(chan struct{})
	words := sourceFunc{name: "words", find: func(context.Context, Query) (Found, error) {
		<-release
		return Found{YRC: "[1000,500](1000,500,0)春(1500,500,0)日影\n"}, nil
	}}
	waited := make(chan struct{})
	s := &Sources{All: []Source{lines, words}, after: func(time.Duration) <-chan time.Time {
		close(waited)
		return make(chan time.Time)
	}}
	done := make(chan []Line, 1)
	go func() {
		var found []Line
		var err error
		found, err = s.Find(context.Background(), spring)
		if err != nil {
			t.Error(err)
		}
		done <- found
	}()
	select {
	case <-waited: // the timed lines are in, and the wait for better begun
	case <-time.After(5 * time.Second):
		t.Fatal("no wait for better lines")
	}
	close(release) // the word times come before it ends
	select {
	case found := <-done:
		if len(found) != 1 || !found[0].Timed {
			t.Fatalf("found %+v", found)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("didn't wait for the word times")
	}
}

// The first source with word times has its lines, with the translation a
// source without them has.
func TestSourcesPreferTimedWords(t *testing.T) {
	lines := &fakeSource{name: "lines", found: Found{LRC: "[00:01.00]春日影\n", Translation: "[00:01.00]春日之影\n"}}
	words := &fakeSource{name: "words", found: Found{YRC: "[1000,500](1000,500,0)春(1500,500,0)日影\n"}}
	s := &Sources{All: []Source{lines, words}}
	found, err := s.Find(context.Background(), spring)
	if err != nil || len(found) != 1 {
		t.Fatalf("found %+v, %v", found, err)
	}
	if !found[0].Timed || found[0].Translation != "春日之影" {
		t.Fatalf("found %+v", found[0])
	}
}

// Lyrics found are kept for good; that a track has none is kept for a
// week, after which the sources are asked again.
func TestSourcesKeepFound(t *testing.T) {
	var has atomic.Bool
	l, asked := lrclib(t, func(w http.ResponseWriter, r *http.Request) {
		if has.Load() {
			reply(w, record{SyncedLyrics: song})
			return
		}
		if r.URL.Path == "/api/search" {
			reply(w, []record{})
			return
		}
		http.NotFound(w, r)
	})
	clock := time.Unix(1000, 0)
	s := &Sources{All: []Source{l}, Cache: t.TempDir(), now: func() time.Time { return clock }}
	find := func() int {
		lines, err := s.Find(context.Background(), spring)
		if err != nil {
			t.Fatal(err)
		}
		return len(lines)
	}
	find()
	n := asked.Load()
	has.Store(true)
	clock = clock.Add(missAgain - time.Hour)
	if find() != 0 || asked.Load() != n {
		t.Fatal("asked again within the week")
	}
	clock = clock.Add(2 * time.Hour)
	if find() != 2 {
		t.Fatal("not asked again after the week")
	}
	n = asked.Load()
	clock = clock.Add(100 * missAgain)
	if find() != 2 || asked.Load() != n {
		t.Fatal("asked again for lyrics found")
	}
}

// A source turned on after a track was found to have no lyrics is asked
// for it.
func TestSourcesAskAgainWhenSourcesChange(t *testing.T) {
	one := &fakeSource{name: "lrclib"}
	two := &fakeSource{name: "netease", found: Found{LRC: "[00:01.00]春日影\n"}}
	cache := t.TempDir()
	clock := time.Unix(1000, 0)
	now := func() time.Time { return clock }
	if lines, err := (&Sources{All: []Source{one}, Cache: cache, now: now}).Find(context.Background(), spring); err != nil || len(lines) != 0 {
		t.Fatalf("found %+v, %v", lines, err)
	}
	lines, err := (&Sources{All: []Source{one, two}, Cache: cache, now: now}).Find(context.Background(), spring)
	if err != nil || len(lines) != 1 || two.asked != 1 {
		t.Fatalf("found %+v, %v, asking netease %d times", lines, err, two.asked)
	}
}

// A source that couldn't be asked isn't kept as the track having no
// lyrics.
func TestSourcesDontKeepErrors(t *testing.T) {
	flaky := &fakeSource{name: "lrclib", err: errors.New("503")}
	cache := t.TempDir()
	s := &Sources{All: []Source{flaky}, Cache: cache}
	if _, err := s.Find(context.Background(), spring); err == nil {
		t.Fatal("no error while down")
	}
	flaky.err, flaky.found = nil, Found{LRC: timeless}
	if lines, err := s.Find(context.Background(), spring); err != nil || len(lines) != 1 {
		t.Fatalf("found %+v, %v once up", lines, err)
	}
	if flaky.asked != 2 {
		t.Fatalf("asked %d times: its error was kept as no lyrics", flaky.asked)
	}
}

// What the cache kept before there were other sources is read as LRCLIB's.
func TestSourcesReadOlderKept(t *testing.T) {
	cache := t.TempDir()
	b := []byte(`{"At":"2026-11-01T00:00:00Z","LRC":"[00:01.00]春日影\n"}`)
	if err := os.WriteFile(filepath.Join(cache, spring.ID+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	lrclib := &fakeSource{name: "lrclib"}
	lines, err := (&Sources{All: []Source{lrclib}, Cache: cache}).Find(context.Background(), spring)
	if err != nil || len(lines) != 1 || lrclib.asked != 0 {
		t.Fatalf("found %+v, %v, asking %d times", lines, err, lrclib.asked)
	}
}

var timeless = "[00:01.00]春日影\n"

// Of tracks as long as the one asked for, to the second, the song itself
// wins over a sped-up cover a little nearer. Spotify's queue gave 麥恩莉
// as 288 seconds, and "麦恩莉(1.2X)", 0.2 seconds off, beat the song
// itself, 0.36 seconds off: its lyrics didn't keep time.
func TestNearestOfPrefersTheSong(t *testing.T) {
	q := Query{Name: "麥恩莉", Album: "回到未來", Duration: 288 * time.Second}
	picks := []pick{
		{Name: "麦恩莉 (Cover 方大同)", Length: 288 * time.Second, HasLyrics: true},
		{Name: "麦恩莉", Album: "回到未来", Length: 288360 * time.Millisecond, HasLyrics: true},
		{Name: "麦恩莉(1.2X)", Length: 288206 * time.Millisecond, HasLyrics: true},
		{Name: "麦恩莉", Length: 288360 * time.Millisecond, HasLyrics: true},
	}
	if i := nearestOf(picks, q, near); i != 1 {
		t.Errorf("chose %q, want the song itself", picks[i].Name)
	}
	// Asked for a live take, a live take is the song.
	live := Query{Name: "麥恩莉 (Live)", Duration: 288 * time.Second}
	picks = []pick{
		{Name: "麦恩莉", Length: 288 * time.Second, HasLyrics: true},
		{Name: "麦恩莉 (Live)", Length: 288 * time.Second, HasLyrics: true},
	}
	if i := nearestOf(picks, live, near); i != 1 {
		t.Errorf("asked for a live take, chose %q", picks[i].Name)
	}
}
