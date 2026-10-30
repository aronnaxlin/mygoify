package app

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/netease"
)

// searchRequest is a search asked of NetEase.
type searchRequest struct {
	query  string
	kind   netease.SearchType
	offset int
}

// fakeSearch replaces searching NetEase with one that reports what it was
// asked, and finds nothing, which lands nowhere without a window.
func fakeSearch(t *testing.T) chan searchRequest {
	asked := make(chan searchRequest, 8)
	searchResults = func(q string, kind netease.SearchType, _, offset int) (netease.SearchResult, error) {
		asked <- searchRequest{q, kind, offset}
		return netease.SearchResult{}, errors.New("offline")
	}
	t.Cleanup(func() { searchResults = netease.SearchPage })
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
	asked := fakeSearch(t)
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("春日影")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if got := wait(t, asked, "the search"); got != (searchRequest{"春日影", netease.SearchSongs, 0}) {
		t.Fatalf("searched %+v", got)
	}

	if err := tt.Click("歌手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := wait(t, asked, "the artists"); got != (searchRequest{"春日影", netease.SearchArtists, 0}) {
		t.Fatalf("searched %+v", got)
	}
	a.search("迷星叫")
	if got := wait(t, asked, "the next search"); got.kind != netease.SearchArtists {
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
	asked := fakeSearch(t)
	a := catalogApp()
	a.open(route{page: pageSearch, query: "MyGO", kind: netease.SearchSongs})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("歌手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.kind != netease.SearchArtists {
		t.Fatalf("on %+v", a.route)
	}
	if len(asked) > 0 {
		t.Error("the artists found were searched for again")
	}
}

// A search's next page loads once its foot comes into view, from where the
// last ended, and a search done again loads from its start.
func TestSearchLoadsMore(t *testing.T) {
	asked := fakeSearch(t)
	a := newTestApp()
	a.settings = defaultSettings()
	r := route{page: pageSearch, query: "春日影", kind: netease.SearchUsers}
	a.searches[r] = &searchData{
		res:  netease.SearchResult{Users: []netease.User{{UserID: 1, Nickname: "甲"}, {UserID: 2, Nickname: "乙"}}, Total: 5},
		page: paged{loaded: true, more: true},
	}
	a.open(r)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if got := wait(t, asked, "the next page"); got != (searchRequest{"春日影", netease.SearchUsers, 2}) {
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
		asked := fakeSearch(t)
		a := newTestApp()
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

// 关注 follows an artist at once, and asks NetEase to; a second click
// waits for the first.
func TestFollowArtist(t *testing.T) {
	asked := make(chan bool, 4)
	followArtist = func(id int64, follow bool) error { asked <- follow; return nil }
	defer func() { followArtist = netease.FollowArtist }()
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1}
	a.artists[12] = &artistData{profile: &netease.ArtistProfile{Artist: netease.Artist{ID: 12, Name: "乙"}}}
	a.open(route{page: pageArtist, id: 12})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("关注"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.artists[12].profile.Followed || !wait(t, asked, "the follow") {
		t.Fatal("not followed")
	}
	tt.Click("已关注")
	if len(asked) > 0 || !a.artists[12].profile.Followed {
		t.Error("unfollowed before the follow landed")
	}
}

// A song row's album opens its page, not playing the song as the row's
// click would.
func TestAlbumLinkInRow(t *testing.T) {
	albumDetail = func(int64) (*netease.AlbumProfile, error) { return nil, errors.New("offline") }
	defer func() { albumDetail = netease.AlbumDetail }()
	a := newTestApp()
	a.settings = defaultSettings()
	song := netease.Track{ID: 2, Name: "起风了", Artists: []netease.Artist{{ID: 13, Name: "丙"}}, Album: netease.Album{ID: 8, Name: "迷跡波"}, Duration: 200000}
	a.artists[13] = &artistData{profile: &netease.ArtistProfile{Artist: netease.Artist{ID: 13, Name: "丙"}, Songs: []netease.Track{song}}}
	a.open(route{page: pageArtist, id: 13})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	// Its name, at the start of the column it fills.
	r, ok := tt.Find("迷跡波")
	if !ok {
		t.Fatal("no album")
	}
	tt.ClickAt(r.X+8, r.Y+r.H/2)
	tt.Frame()
	if a.route != (route{page: pageAlbum, id: 8}) || a.pl.Current() != nil {
		t.Fatalf("opened %+v, playing %+v", a.route, a.pl.Current())
	}
}

// catalogApp is an app with an artist, album, user and podcast loaded, and
// searches for each kind, to look at.
func catalogApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	day := time.Date(2026, 10, 1, 20, 0, 0, 0, time.Local).UnixMilli()
	band := netease.Artist{ID: 7, Name: "MyGO!!!!!"}
	al := netease.Album{ID: 8, Name: "迷跡波"}
	var songs []netease.Track
	for i, name := range []string{"春日影", "迷星叫", "壱雫空", "碧天伴走", "音一会", "栞"} {
		songs = append(songs, netease.Track{ID: int64(i + 1), Name: name, Artists: []netease.Artist{band}, Album: al, Duration: int64(210000 + 16000*i)})
	}
	var albums []netease.AlbumProfile
	for i, name := range []string{"迷跡波", "跡暖空", "壱雫空", "音一会", "迷星叫", "焚音打"} {
		albums = append(albums, netease.AlbumProfile{Album: netease.Album{ID: int64(20 + i), Name: name}, Artists: []netease.Artist{band}, PublishTime: day})
	}
	a.artists[7] = &artistData{
		profile: &netease.ArtistProfile{Artist: band, Alias: []string{"マイゴ"}, MusicSize: 38, AlbumSize: 9, BriefDesc: "迷いながらでも、前へ。", AccountID: 70, Songs: songs},
		albums:  albums, albumsPage: paged{loaded: true},
	}
	a.albums[8] = &albumData{album: &netease.AlbumProfile{Album: al, Artists: []netease.Artist{band}, PublishTime: day, Description: "MyGO!!!!! 的第一张专辑。", Songs: songs}}
	for _, res := range []struct {
		kind netease.SearchType
		res  netease.SearchResult
	}{
		{netease.SearchSongs, netease.SearchResult{Songs: songs, Total: 38}},
		{netease.SearchArtists, netease.SearchResult{Artists: []netease.ArtistProfile{*a.artists[7].profile, {Artist: netease.Artist{ID: 10, Name: "Ave Mujica"}}}, Total: 2}},
		{netease.SearchAlbums, netease.SearchResult{Albums: albums, Total: 6}},
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
	{"search-songs", route{page: pageSearch, query: "MyGO", kind: netease.SearchSongs}, 0},
	{"search-artists", route{page: pageSearch, query: "MyGO", kind: netease.SearchArtists}, 0},
	{"search-albums", route{page: pageSearch, query: "MyGO", kind: netease.SearchAlbums}, 0},
	{"artist", route{page: pageArtist, id: 7}, artistHot},
	{"artist-albums", route{page: pageArtist, id: 7}, artistAlbums},
	{"album", route{page: pageAlbum, id: 8}, 0},
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
