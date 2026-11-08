package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// The playlists Spotify made for the listener, as Discover Weekly, the
// Daily Mixes and the daylist (music.IsMix), the Web API neither lists nor
// reads, but Soloist plays. The app keeps those it sees, as the listener
// played them lately or plays them here, and shows them on the home page
// (DESIGN.md §5.4).

// mix is a playlist Spotify made for the listener, as the app saw it.
type mix struct {
	URI string `json:"uri"`
	// Name is Soloist's name for it as it last played: the daylist's of
	// the time of day, which oEmbed calls "daylist".
	Name string `json:"name,omitempty"`
	// Title and Images are oEmbed's, as of Asked.
	Title  string       `json:"title,omitempty"`
	Images music.Images `json:"images,omitempty"`
	Asked  time.Time    `json:"asked,omitzero"`
	Seen   time.Time    `json:"seen"`
}

func (m *mix) id() string { return strings.TrimPrefix(m.URI, "spotify:playlist:") }

// name is what the mix is called: as it last played, or as oEmbed says.
func (m *mix) name() string {
	if m.Name != "" {
		return m.Name
	}
	return m.Title
}

// mixes are the mixes the app saw, the latest found first, and the file
// of the listener's they are kept in, if any. Those seen again keep their
// place: a card clicked stays under the pointer.
type mixes struct {
	list   []mix
	file   string
	asking map[string]bool
}

const (
	// maxMixes is how many mixes the app keeps.
	maxMixes = 12
	// mixGone is how long a mix unseen is kept: a Daily Mix Spotify
	// stopped making goes.
	mixGone = 30 * 24 * time.Hour
	// mixAskAgain is how long oEmbed's name and cover for a mix stay
	// good: Spotify makes new covers as it changes the mixes.
	mixAskAgain = 24 * time.Hour
)

// loadMixes reads the mixes kept for the listener of id.
func (a *app) loadMixes(id string) {
	a.mixes = mixes{}
	if !a.live || id == "" {
		return
	}
	a.mixes.file = filepath.Join(dataDir(), "mixes", id+".json")
	if b, err := os.ReadFile(a.mixes.file); err == nil {
		json.Unmarshal(b, &a.mixes.list)
	}
	for i := range a.mixes.list {
		a.askMix(&a.mixes.list[i])
	}
}

func (a *app) saveMixes() {
	f := a.mixes.file
	if f == "" {
		return
	}
	b, err := json.Marshal(a.mixes.list)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(f), 0o700)
	tmp := f + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, f)
	}
}

// seeMix keeps that the mix uri was seen at, named name by Soloist, if
// set, and asks its name and cover of the library if they are stale.
func (a *app) seeMix(uri, name string, at time.Time) {
	if !music.IsMix(uri) {
		return
	}
	l := &a.mixes.list
	i := slices.IndexFunc(*l, func(m mix) bool { return m.URI == uri })
	if i < 0 {
		*l = slices.Insert(*l, 0, mix{URI: uri})
		i = 0
	}
	m := &(*l)[i]
	changed := m.Seen.IsZero() || name != "" && name != m.Name || at.Sub(m.Seen) > time.Hour
	if name != "" {
		m.Name = name
	}
	if at.After(m.Seen) {
		m.Seen = at
	}
	*l = slices.DeleteFunc(*l, func(m mix) bool { return at.Sub(m.Seen) > mixGone })
	if len(*l) > maxMixes {
		*l = (*l)[:maxMixes]
	}
	if changed {
		a.saveMixes()
	}
	if i := slices.IndexFunc(*l, func(m mix) bool { return m.URI == uri }); i >= 0 {
		a.askMix(&(*l)[i])
	}
}

// seeHomeMixes keeps the mixes the listener played lately, given the
// latest first: found anew, they come first in that order.
func (a *app) seeHomeMixes(uris []string, at time.Time) {
	for _, uri := range slices.Backward(uris) {
		a.seeMix(uri, "", at)
	}
}

// askMix asks the library for m's name and cover, unless it answered of
// late.
func (a *app) askMix(m *mix) {
	if time.Since(m.Asked) < mixAskAgain || a.mixes.asking[m.URI] {
		return
	}
	if a.mixes.asking == nil {
		a.mixes.asking = map[string]bool{}
	}
	uri, id := m.URI, m.id()
	a.mixes.asking[uri] = true
	go func() {
		pl, err := a.lib.Mix(id)
		a.post(func() {
			delete(a.mixes.asking, uri)
			i := slices.IndexFunc(a.mixes.list, func(m mix) bool { return m.URI == uri })
			if err != nil || i < 0 {
				return
			}
			m := &a.mixes.list[i]
			m.Title, m.Images, m.Asked = pl.Name, pl.Images, time.Now()
			a.saveMixes()
		})
	}()
}

// mixCards shows the mixes the app saw, as many as fit: a click anywhere
// on one plays it from its start.
func (a *app) mixCards(c *ui.Context) {
	cols := a.columns(c, 168)
	list := a.mixes.list
	ui.Grid(c).Columns(cols).GapX(20).GapY(28).Children(func() {
		for _, m := range list[:min(len(list), cols)] {
			play := func() { a.pl.PlayQueue(m.URI, nil, 0) }
			if a.coverCard(c.Key(m.URI), m.name(), m.Images, "", play) {
				play()
			}
		}
	})
}
