package soloist

import (
	"testing"

	"mygoify/internal/playback"
)

// The state names what plays as Soloist names it once it plays it: the
// event that the context changed comes without a name, and another
// context drops the name of the one before.
func TestContextName(t *testing.T) {
	f := newFakeSoloist(t)
	b := New(Config{DataDir: f.dir})
	states := watch(b)
	f.connected(t, 1)
	dw := `{"uri":"spotify:playlist:37i9dQZEVXcJIKCgVG2l9T","entity_type":"playlist","decorations":{"identity":{"name":"Discover Weekly"}}}`
	f.send(t, `{"type":"context_changed","context":{"uri":"spotify:playlist:37i9dQZEVXcJIKCgVG2l9T","entity_type":"playlist","decorations":{}}}`)
	f.send(t, `{"type":"playback_state","status":"playing","item":`+track("a", "a")+`,"context":`+dw+`}`)
	until(t, states, func(st playback.State) bool { return st.ContextName == "Discover Weekly" })
	f.send(t, `{"type":"context_changed","context":{"uri":"spotify:playlist:37i9dQZF1E39i7UAVcxDHE","entity_type":"playlist","decorations":{}}}`)
	st := until(t, states, func(st playback.State) bool { return st.Context == "spotify:playlist:37i9dQZF1E39i7UAVcxDHE" })
	if st.ContextName != "" {
		t.Fatalf("the Daily Mix is named %q", st.ContextName)
	}
}
