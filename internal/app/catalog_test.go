package app

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
)

// searchRequest is a search asked of Spotify.
type searchRequest struct {
	query  string
	kind   music.SearchType
	offset int
}

// fakeSearch makes a's searches report what they were asked, and find
// nothing, which lands nowhere without a window.
func fakeSearch(a *app) chan searchRequest {
	asked := make(chan searchRequest, 8)
	stub(a).search = func(q string, kind music.SearchType, _, offset int) (music.SearchResult, error) {
		asked <- searchRequest{q, kind, offset}
		return music.SearchResult{}, errors.New("offline")
	}
	return asked
}

func wait[T any](t *testing.T, ch chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(time.Second):
		t.Fatalf("%s wasn't asked for", what)
		panic("unreachable")
	}
}

// Enter searches songs for the query typed; a tab searches it for another
// kind, which the next search keeps, and back leaves the search.
func TestSearchKinds(t *testing.T) {
	a := newTestApp()
	asked := fakeSearch(a)
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("春日影")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if got := wait(t, asked, "the search"); got != (searchRequest{"春日影", music.SearchTracks, 0}) {
		t.Fatalf("searched %+v", got)
	}

	if err := tt.Click("歌手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := wait(t, asked, "the artists"); got != (searchRequest{"春日影", music.SearchArtists, 0}) {
		t.Fatalf("searched %+v", got)
	}
	a.search("迷星叫")
	if got := wait(t, asked, "the next search"); got.kind != music.SearchArtists {
		t.Errorf("the next search looked for kind %d", got.kind)
	}
	if !slices.Equal(a.settings.SearchHistory, []string{"迷星叫", "春日影"}) {
		t.Errorf("history %q", a.settings.SearchHistory)
	}

	tt.Key(0, ui.KeyBack)
	tt.Key(0, ui.KeyBack)
	if a.route.page != pageHome {
		t.Errorf("back went to %+v", a.route)
	}
}

// The tabs over a search's songs switch it to another kind: it crashed,
// the list's state reset as the list built the tabs in its header.
func TestSearchKindFromSongs(t *testing.T) {
	a := catalogApp()
	asked := fakeSearch(a)
	a.open(route{page: pageSearch, query: "MyGO", kind: music.SearchTracks})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("歌手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.kind != music.SearchArtists {
		t.Fatalf("on %+v", a.route)
	}
	if len(asked) > 0 {
		t.Error("the artists found were searched for again")
	}
}

// A search's next page loads once its foot comes into view, from where the
// last ended, past what it left out, and a search done again loads from
// its start. Paged from what showed, playlists taken down made results
// show twice.
func TestSearchLoadsMore(t *testing.T) {
	a := newTestApp()
	asked := fakeSearch(a)
	a.settings = defaultSettings()
	r := route{page: pageSearch, query: "春日影", kind: music.SearchPlaylists}
	a.searches[r] = &searchData{
		res:  music.SearchResult{Playlists: []music.Playlist{{ID: "1", Name: "甲"}, {ID: "2", Name: "乙"}}, More: true, Skipped: 1},
		page: paged{loaded: true, more: true},
	}
	a.open(r)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if got := wait(t, asked, "the next page"); got != (searchRequest{"春日影", music.SearchPlaylists, 3}) {
		t.Fatalf("asked for %+v", got)
	}

	a.search("春日影")
	if got := wait(t, asked, "the search again"); got.offset != 0 {
		t.Errorf("searched again from %d", got.offset)
	}
}

// A search before, from the empty field's popup, searches again, clicked
// or chosen with the arrows; it was lost when its option was keyed anew.
func TestSearchHistoryChoice(t *testing.T) {
	for _, keys := range []bool{false, true} {
		a := newTestApp()
		asked := fakeSearch(a)
		a.settings = defaultSettings()
		a.settings.SearchHistory = []string{"春日影", "迷星叫"}
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		if err := tt.Click("搜索"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if keys {
			tt.Key(0, ui.KeyDown)
			tt.Key(0, ui.KeyDown)
			tt.Key(0, ui.KeyEnter)
		} else if err := tt.Click("迷星叫"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := wait(t, asked, "the search"); got.query != "迷星叫" || a.query != "迷星叫" {
			t.Fatalf("keys %v: searched %+v, field %q", keys, got, a.query)
		}
		if !slices.Equal(a.settings.SearchHistory, []string{"迷星叫", "春日影"}) {
			t.Errorf("keys %v: history %q", keys, a.settings.SearchHistory)
		}
	}
}

// 关注 follows an artist at once, and asks Spotify to; a second click
// waits for the first.
func TestFollowArtist(t *testing.T) {
	asked := make(chan bool, 4)
	a := newTestApp()
	stub(a).follow = func(id string, follow bool) error { asked <- follow; return nil }
	a.settings = defaultSettings()
	a.profile = &music.User{ID: "1"}
	a.artists["12"] = &artistData{profile: &music.ArtistProfile{Artist: music.Artist{ID: "12", Name: "乙"}}}
	a.open(route{page: pageArtist, id: "12"})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("关注"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.artists["12"].profile.Followed || !wait(t, asked, "the follow") {
		t.Fatal("not followed")
	}
	tt.Click("已关注")
	if len(asked) > 0 || !a.artists["12"].profile.Followed {
		t.Error("unfollowed before the follow landed")
	}
}

// A song row's album opens its page, not playing the song as the row's
// click would.
func TestAlbumLinkInRow(t *testing.T) {
	a := newTestApp()
	stub(a).album = func(string) (*music.AlbumProfile, error) { return nil, errors.New("offline") }
	a.settings = defaultSettings()
	song := music.Track{ID: "2", Name: "起风了", Artists: []music.Artist{{ID: "13", Name: "丙"}}, Album: music.Album{ID: "8", Name: "迷跡波"}, Duration: 200000}
	a.artists["13"] = &artistData{profile: &music.ArtistProfile{Artist: music.Artist{ID: "13", Name: "丙"}, Songs: []music.Track{song}}}
	a.open(route{page: pageArtist, id: "13"})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	// Its name, at the start of the column it fills.
	r, ok := tt.Find("迷跡波")
	if !ok {
		t.Fatal("no album")
	}
	tt.ClickAt(r.X+8, r.Y+r.H/2)
	tt.Frame()
	if a.route != (route{page: pageAlbum, id: "8"}) || a.pl.Current() != nil {
		t.Fatalf("opened %+v, playing %+v", a.route, a.pl.Current())
	}
}

// catalogApp is an app with an artist and an album loaded, and searches
// for each kind, to look at.
func catalogApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	band := music.Artist{ID: "7", Name: "MyGO!!!!!"}
	al := music.Album{ID: "8", Name: "迷跡波"}
	var songs []music.Track
	for i, name := range []string{"春日影", "迷星叫", "壱雫空", "碧天伴走", "音一会", "栞"} {
		songs = append(songs, music.Track{ID: fmt.Sprint(i + 1), Name: name, Artists: []music.Artist{band}, Album: al, Duration: int64(210000 + 16000*i)})
	}
	var albums []music.AlbumProfile
	for i, name := range []string{"迷跡波", "跡暖空", "壱雫空", "音一会", "迷星叫", "焚音打"} {
		albums = append(albums, music.AlbumProfile{Album: music.Album{ID: fmt.Sprint(20 + i), Name: name}, Artists: []music.Artist{band}, Released: "2023-12-13"})
	}
	a.artists["7"] = &artistData{
		profile: &music.ArtistProfile{Artist: band, Genres: []string{"j-rock", "anime"}, Songs: songs},
		albums:  albums, albumsPage: paged{loaded: true},
	}
	a.albums["8"] = &albumData{album: &music.AlbumProfile{Album: al, Artists: []music.Artist{band}, Released: "2023-12-13", Label: "BanG Dream!", Songs: songs}}
	for _, res := range []struct {
		kind music.SearchType
		res  music.SearchResult
	}{
		{music.SearchTracks, music.SearchResult{Tracks: songs, More: true}},
		{music.SearchArtists, music.SearchResult{Artists: []music.ArtistProfile{*a.artists["7"].profile, {Artist: music.Artist{ID: "10", Name: "Ave Mujica"}}}}},
		{music.SearchAlbums, music.SearchResult{Albums: albums}},
	} {
		a.searches[route{page: pageSearch, query: "MyGO", kind: res.kind}] = &searchData{res: res.res, page: paged{loaded: true}}
	}
	return a
}

// catalogScenes are the pages of catalogApp.
var catalogScenes = []struct {
	name string
	r    route
	tab  int
}{
	{"search-songs", route{page: pageSearch, query: "MyGO", kind: music.SearchTracks}, 0},
	{"search-artists", route{page: pageSearch, query: "MyGO", kind: music.SearchArtists}, 0},
	{"search-albums", route{page: pageSearch, query: "MyGO", kind: music.SearchAlbums}, 0},
	{"artist", route{page: pageArtist, id: "7"}, artistHot},
	{"artist-albums", route{page: pageArtist, id: "7"}, artistAlbums},
	{"album", route{page: pageAlbum, id: "8"}, 0},
}

// With NM_FRAMES set, saves the pages of searches, artists and albums in
// the light and the dark.
func TestCatalogFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, s := range catalogScenes {
		for _, dark := range []bool{false, true} {
			a := catalogApp()
			a.artistTab = s.tab
			a.open(s.r)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(time.Second)
			tt.Frame()
			name := s.name
			if dark {
				name += "-dark"
			}
			f, _ := os.Create(filepath.Join(dir, "catalog-"+name+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}

// The pages of searches, artists, albums, users and podcasts ask for no
// frames once shown.
func TestCatalogFrameBudget(t *testing.T) {
	for _, s := range catalogScenes {
		a := catalogApp()
		a.artistTab = s.tab
		a.open(s.r)
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Advance(time.Second)
		tt.Frame()
		tt.Advance(time.Second)
		if tt.FramePending() {
			t.Errorf("%s asks for frames while idle", s.name)
		}
	}
}
