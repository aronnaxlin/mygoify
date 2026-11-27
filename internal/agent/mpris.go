package agent

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"

	"mygoify/internal/music"
	"mygoify/internal/playback"
)

// The names MyGoify's MPRIS player is known by. The bus name and the
// desktop entry are the app's, so that desktops show its icon and name.
const (
	busName      = "org.mpris.MediaPlayer2.mygoify"
	mprisPath    = "/org/mpris/MediaPlayer2"
	rootIface    = "org.mpris.MediaPlayer2"
	playerIface  = "org.mpris.MediaPlayer2.Player"
	desktopEntry = "mygoify"
	identity     = "MyGoify"
	trackPrefix  = "/io/github/aronnaxlin/mygoify/track/"
	noTrack      = "/org/mpris/MediaPlayer2/TrackList/NoTrack"
)

// Controls is what the MPRIS player asks of what plays.
type Controls interface {
	Resume() error
	Pause() error
	Next() error
	Prev() error
	Seek(time.Duration) error
	SetVolume(float64) error
	SetShuffle(bool) error
	SetRepeat(playback.RepeatMode) error
}

// mpris is MyGoify's MPRIS player on the session bus, which media keys,
// playerctl and the desktop's media controls drive, Omarchy's among them.
type mpris struct {
	ctl   Controls
	raise func() // opens the app's window
	conn  *dbus.Conn
	props *prop.Properties

	mu     sync.Mutex
	st     playback.State
	track  string // the ID of the track the properties are of
	volume float64
}

// exportMPRIS puts the player on the session bus.
func exportMPRIS(conn *dbus.Conn, ctl Controls, volume float64, raise func()) (*mpris, error) {
	m := &mpris{ctl: ctl, raise: raise, conn: conn, volume: volume, st: playback.State{At: -1}}
	if err := conn.Export(m, mprisPath, rootIface); err != nil {
		return nil, err
	}
	// Seek is SeekBy in Go, where vet takes Seek for io.Seeker's.
	if err := conn.ExportWithMap(player{m}, map[string]string{"SeekBy": "Seek"}, mprisPath, playerIface); err != nil {
		return nil, err
	}
	props, err := prop.Export(conn, mprisPath, prop.Map{
		rootIface: {
			"CanQuit":             {Value: false},
			"CanRaise":            {Value: true},
			"HasTrackList":        {Value: false},
			"Identity":            {Value: identity},
			"DesktopEntry":        {Value: desktopEntry},
			"SupportedUriSchemes": {Value: []string{}},
			"SupportedMimeTypes":  {Value: []string{}},
		},
		playerIface: {
			"PlaybackStatus": {Value: "Stopped", Emit: prop.EmitTrue},
			"LoopStatus":     {Value: "None", Writable: true, Emit: prop.EmitTrue, Callback: m.setLoop},
			"Rate":           {Value: 1.0, Emit: prop.EmitTrue},
			"Shuffle":        {Value: false, Writable: true, Emit: prop.EmitTrue, Callback: m.setShuffle},
			"Metadata":       {Value: map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath(noTrack))}, Emit: prop.EmitTrue},
			"Volume":         {Value: volume, Writable: true, Emit: prop.EmitTrue, Callback: m.setVolume},
			"Position":       {Value: int64(0), Emit: prop.EmitFalse},
			"MinimumRate":    {Value: 1.0},
			"MaximumRate":    {Value: 1.0},
			"CanGoNext":      {Value: false, Emit: prop.EmitTrue},
			"CanGoPrevious":  {Value: false, Emit: prop.EmitTrue},
			"CanPlay":        {Value: false, Emit: prop.EmitTrue},
			"CanPause":       {Value: false, Emit: prop.EmitTrue},
			"CanSeek":        {Value: false, Emit: prop.EmitTrue},
			"CanControl":     {Value: true},
		},
	})
	if err != nil {
		return nil, err
	}
	m.props = props
	node := &introspect.Node{
		Name: mprisPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{Name: rootIface, Methods: introspect.Methods(m), Properties: props.Introspection(rootIface)},
			{Name: playerIface, Methods: playerMethods(m), Properties: props.Introspection(playerIface),
				Signals: []introspect.Signal{{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}}}},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), mprisPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, err
	}
	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return nil, fmt.Errorf("agent: %s is taken: another agent runs", busName)
	}
	return m, nil
}

// Raise opens the app's window.
func (m *mpris) Raise() *dbus.Error {
	if m.raise != nil {
		m.raise()
	}
	return nil
}

// Quit does nothing: CanQuit says so.
func (m *mpris) Quit() *dbus.Error { return nil }

// player is the Player interface's methods, apart from the root's.
type player struct{ m *mpris }

func dbusErr(err error) *dbus.Error {
	if err == nil {
		return nil
	}
	return dbus.MakeFailedError(err)
}

func (p player) Next() *dbus.Error     { return dbusErr(p.m.ctl.Next()) }
func (p player) Previous() *dbus.Error { return dbusErr(p.m.ctl.Prev()) }
func (p player) Pause() *dbus.Error    { return dbusErr(p.m.ctl.Pause()) }
func (p player) Play() *dbus.Error     { return dbusErr(p.m.ctl.Resume()) }
func (p player) Stop() *dbus.Error     { return dbusErr(p.m.ctl.Pause()) }

func (p player) PlayPause() *dbus.Error {
	p.m.mu.Lock()
	playing := p.m.st.Playing
	p.m.mu.Unlock()
	if playing {
		return p.Pause()
	}
	return p.Play()
}

// playerMethods are the Player interface's methods as D-Bus names them.
func playerMethods(m *mpris) []introspect.Method {
	methods := introspect.Methods(player{m})
	for i := range methods {
		if methods[i].Name == "SeekBy" {
			methods[i].Name = "Seek"
		}
	}
	return methods
}

// SeekBy, MPRIS's Seek, moves by offset, in microseconds; past the end,
// it goes to the next track, as MPRIS says.
func (p player) SeekBy(offset int64) *dbus.Error {
	m := p.m
	m.mu.Lock()
	tr, pos := current(m.st), position(m.st, time.Now())
	m.mu.Unlock()
	if tr == nil {
		return nil
	}
	to := pos + time.Duration(offset)*time.Microsecond
	if to >= tr.Length() {
		return p.Next()
	}
	return dbusErr(m.ctl.Seek(max(0, to)))
}

// SetPosition goes to pos, in microseconds, of the track trackID names, if
// it is still the one playing.
func (p player) SetPosition(trackID dbus.ObjectPath, pos int64) *dbus.Error {
	m := p.m
	m.mu.Lock()
	tr := current(m.st)
	m.mu.Unlock()
	if tr == nil || trackID != trackPath(tr.ID) || pos < 0 || time.Duration(pos)*time.Microsecond > tr.Length() {
		return nil
	}
	return dbusErr(m.ctl.Seek(time.Duration(pos) * time.Microsecond))
}

// OpenUri plays nothing: SupportedUriSchemes is empty.
func (p player) OpenUri(string) *dbus.Error { return nil }

func (m *mpris) setLoop(c *prop.Change) *dbus.Error {
	r := playback.RepeatOff
	switch c.Value.(string) {
	case "Track":
		r = playback.RepeatOne
	case "Playlist":
		r = playback.RepeatAll
	}
	return dbusErr(m.ctl.SetRepeat(r))
}

func (m *mpris) setShuffle(c *prop.Change) *dbus.Error {
	return dbusErr(m.ctl.SetShuffle(c.Value.(bool)))
}

func (m *mpris) setVolume(c *prop.Change) *dbus.Error {
	v := max(0, min(1, c.Value.(float64)))
	m.mu.Lock()
	m.volume = v
	m.mu.Unlock()
	return dbusErr(m.ctl.SetVolume(v))
}

// current is the track st plays, or nil.
func current(st playback.State) *music.Track {
	if st.At < 0 || st.At >= len(st.Queue) {
		return nil
	}
	return &st.Queue[st.At]
}

// position is where st's track is at now, moving on while it plays.
func position(st playback.State, now time.Time) time.Duration {
	pos := st.Position
	if st.Playing && !st.Loading && !st.Since.IsZero() {
		pos += now.Sub(st.Since)
	}
	if tr := current(st); tr != nil && tr.Duration > 0 {
		pos = min(pos, tr.Length())
	}
	return max(0, pos)
}

// trackPath is the object path MPRIS names a track by. A Spotify ID is
// letters and digits, which a path takes.
func trackPath(id string) dbus.ObjectPath {
	return dbus.ObjectPath(trackPrefix + strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, id))
}

// metadata is MPRIS's metadata of t.
func metadata(t *music.Track) map[string]dbus.Variant {
	if t == nil {
		return map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath(noTrack))}
	}
	md := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(trackPath(t.ID)),
		"mpris:length":  dbus.MakeVariant(t.Length().Microseconds()),
		"xesam:title":   dbus.MakeVariant(t.Name),
		"xesam:url":     dbus.MakeVariant(t.URL()),
	}
	if len(t.Artists) > 0 {
		names := make([]string, len(t.Artists))
		for i, a := range t.Artists {
			names[i] = a.Name
		}
		md["xesam:artist"] = dbus.MakeVariant(names)
	}
	if t.Album.Name != "" {
		md["xesam:album"] = dbus.MakeVariant(t.Album.Name)
	}
	if u := t.Album.Images.URL(640); u != "" {
		md["mpris:artUrl"] = dbus.MakeVariant(u)
	}
	return md
}

// status is MPRIS's PlaybackStatus of st.
func status(st playback.State) string {
	switch {
	case current(st) == nil:
		return "Stopped"
	case st.Playing:
		return "Playing"
	}
	return "Paused"
}

// seekedBy is how far the position may stray from where it was heading
// before the player says it was moved: Soloist's reports wander a little.
const seekedBy = 1500 * time.Millisecond

// update shows st: what plays, how, and where, saying Seeked when the
// track playing was moved within.
func (m *mpris) update(st playback.State) {
	now := time.Now()
	m.mu.Lock()
	was, wasPos := m.st, position(m.st, now)
	m.st = st
	m.mu.Unlock()
	tr := current(st)
	id := ""
	if tr != nil {
		id = tr.ID
	}
	// SetMust, as Set is the bus's, which refuses the read-only.
	set := func(name string, v any) { m.props.SetMust(playerIface, name, v) }
	if id != m.track {
		m.track = id
		m.props.SetMust(playerIface, "Metadata", metadata(tr))
	}
	m.props.SetMust(playerIface, "Position", position(st, now).Microseconds())
	if s := status(st); s != m.props.GetMust(playerIface, "PlaybackStatus").(string) {
		set("PlaybackStatus", s)
	}
	loop := map[playback.RepeatMode]string{playback.RepeatOff: "None", playback.RepeatOne: "Track", playback.RepeatAll: "Playlist"}[st.Repeat]
	for name, v := range map[string]any{
		"LoopStatus":    loop,
		"Shuffle":       st.Shuffle,
		"CanGoNext":     tr != nil,
		"CanGoPrevious": tr != nil,
		"CanPlay":       tr != nil,
		"CanPause":      tr != nil,
		"CanSeek":       tr != nil && tr.Duration > 0,
	} {
		if m.props.GetMust(playerIface, name) != v {
			set(name, v)
		}
	}
	if tr != nil && id == trackID(was) {
		if pos := position(st, now); pos-wasPos > seekedBy || wasPos-pos > seekedBy {
			if err := m.conn.Emit(mprisPath, playerIface+".Seeked", pos.Microseconds()); err != nil {
				log.Printf("agent: mpris Seeked: %v", err)
			}
		}
	}
}

func trackID(st playback.State) string {
	if tr := current(st); tr != nil {
		return tr.ID
	}
	return ""
}
